package com.orb.app

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.AudioAttributes
import android.media.AudioDeviceInfo
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.media.audiofx.Equalizer
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.annotation.OptIn
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.session.CommandButton
import androidx.media3.session.DefaultMediaNotificationProvider
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService
import androidx.media3.session.SessionCommand
import androidx.media3.session.SessionResult
import com.google.common.collect.ImmutableList
import com.google.common.util.concurrent.Futures
import com.google.common.util.concurrent.ListenableFuture
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors

@OptIn(UnstableApi::class)
class MediaService : MediaSessionService() {

    private var player: ExoPlayer? = null
    private var wrappedPlayer: ForwardingPlayer? = null
    private var mediaSession: MediaSession? = null

    // Custom command identifiers
    private val SHUFFLE_COMMAND = "SHUFFLE_TOGGLE"
    private val FAVORITE_COMMAND = "FAVORITE_TOGGLE"
    private val AB_SKIP_BACK_15 = "AB_SKIP_BACK_15"
    private val AB_SKIP_FORWARD_15 = "AB_SKIP_FORWARD_15"
    private val AB_SPEED_CYCLE = "AB_SPEED_CYCLE"
    private val AB_CHAPTER_START = "AB_CHAPTER_START"
    private val POD_SKIP_BACK_15 = "POD_SKIP_BACK_15"
    private val POD_SKIP_FORWARD_30 = "POD_SKIP_FORWARD_30"
    private val POD_SPEED_CYCLE = "POD_SPEED_CYCLE"

    // Icon resources for different playback speeds (0.5x to 2x in 0.1 increments)
    private val speedIcons = mapOf(
        0.5f to R.drawable.ic_playback_speed_0_point_5x,
        0.6f to R.drawable.ic_playback_speed_0_point_6x,
        0.7f to R.drawable.ic_playback_speed_0_point_7x,
        0.8f to R.drawable.ic_playback_speed_0_point_8x,
        0.9f to R.drawable.ic_playback_speed_0_point_9x,
        1.0f to R.drawable.ic_playback_speed_1,
        1.1f to R.drawable.ic_playback_speed_1_point_1x,
        1.2f to R.drawable.ic_playback_speed_1_point_2x,
        1.3f to R.drawable.ic_playback_speed_1_point_3x,
        1.4f to R.drawable.ic_playback_speed_1_point_4x,
        1.5f to R.drawable.ic_playback_speed_1_point_5x,
        1.6f to R.drawable.ic_playback_speed_1_point_6x,
        1.7f to R.drawable.ic_playback_speed_1_point_7x,
        1.8f to R.drawable.ic_playback_speed_1_point_8x,
        1.9f to R.drawable.ic_playback_speed_1_point_9x,
        2.0f to R.drawable.ic_playback_speed_2
    )

    // Current playback speed, used to update the notification icon. Cached here since ExoPlayer doesn't provide a callback for speed changes.
    @Volatile private var currentSpeed = 1.0f

    // State (synced with frontend)
    private var isShuffled = false
    private var isFavorited = false
    @Volatile private var isAudiobook = false
    @Volatile private var isPodcast = false

    // ── Equalizer ────────────────────────────────────────────────────────────
    private var equalizer: Equalizer? = null

    // ── Crossfade ────────────────────────────────────────────────────────────
    /** Secondary ExoPlayer used to keep the old track audible during a crossfade. */
    private var crossfadePlayer: ExoPlayer? = null
    /** Pending step-wise volume animation runnable. */
    private var crossfadeRunnable: Runnable? = null
    @Volatile private var crossfadeEnabled = false
    @Volatile private var crossfadeSecs = 3f
    /**
     * Set to true when we fire nativeOnNext() early (crossfade trigger) so
     * STATE_ENDED on the main player doesn't double-fire the event.
     */
    @Volatile private var crossfadeTriggered = false

    // ── Native queue preloading ───────────────────────────────────────────────
    // When the JS layer preloads the next track, we store it here so that
    // STATE_ENDED can advance playback without waiting for the WebView to wake.
    @Volatile private var pendingNextUrl: String? = null
    @Volatile private var pendingNextTitle: String? = null
    @Volatile private var pendingNextArtist: String? = null
    @Volatile private var pendingNextCover: String? = null
    /**
     * Set to true after native auto-advances to the preloaded next track.
     * The subsequent JS play_music call (when the WebView wakes) is suppressed
     * so we don't restart the track that is already playing.
     */
    @Volatile private var nativeAutoAdvanced = false

    // Audio focus: tracks whether we paused due to a transient focus loss so we
    // can resume automatically when focus returns.
    @Volatile private var pausedByFocusLoss = false
    private var audioFocusRequest: AudioFocusRequest? = null  // API 26+

    // Pause on permanent loss (another media app) or transient loss (phone call,
    // navigation). Ignore AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK so game audio can
    // play alongside music without interruption.
    private val audioFocusListener = AudioManager.OnAudioFocusChangeListener { focusChange ->
        when (focusChange) {
            AudioManager.AUDIOFOCUS_LOSS -> {
                // Another media app has taken focus permanently — pause and do not
                // auto-resume; the user chose to switch apps.
                pausedByFocusLoss = false
                player?.pause()
            }
            AudioManager.AUDIOFOCUS_LOSS_TRANSIENT -> {
                // Temporary interruption (e.g. phone call) — pause and resume when
                // focus returns.
                if (player?.isPlaying == true) {
                    pausedByFocusLoss = true
                    player?.pause()
                }
            }
            AudioManager.AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK -> {
                // Another app (typically a game or navigation prompt) requested focus
                // with MAY_DUCK — allow it to play concurrently without pausing or
                // lowering our volume.
            }
            AudioManager.AUDIOFOCUS_GAIN -> {
                // Focus returned — resume only if we paused due to a transient loss.
                if (pausedByFocusLoss) {
                    pausedByFocusLoss = false
                    player?.play()
                }
            }
        }
    }

    // Cached position/duration — updated on the main thread, safely readable from JNI threads.
    @Volatile private var cachedPosition: Long = 0L
    @Volatile private var cachedDuration: Long = 0L

    private val mainHandler = Handler(Looper.getMainLooper())
    private val positionUpdater = object : Runnable {
        override fun run() {
            player?.let {
                cachedPosition = it.currentPosition
                cachedDuration = it.duration.coerceAtLeast(0L)

                // Crossfade early-advance: fire nativeOnNext() when crossfadeSecs remain
                // so the frontend can queue the next track before the current one ends.
                if (crossfadeEnabled && crossfadeSecs > 0 && !crossfadeTriggered && cachedDuration > 0) {
                    val remaining = cachedDuration - cachedPosition
                    val triggerMs = (crossfadeSecs * 1000f).toLong().coerceAtLeast(500L)
                    if (remaining in 1L..triggerMs) {
                        crossfadeTriggered = true
                        try { nativeOnNext() } catch (_: Exception) {}
                    }
                }
            }
            mainHandler.postDelayed(this, 200)
        }
    }

    private var lastNotifiedVolume: Float = -1f

    // Poll system music volume every 250 ms. ExoPlayer's onDeviceVolumeChanged
    // is unreliable for hardware button presses when the player is paused or in
    // the background, so this loop guarantees the slider stays in sync.
    private val volumeChecker = object : Runnable {
        override fun run() {
            val am = getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val current = am.getStreamVolume(AudioManager.STREAM_MUSIC)
            val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            val normalized = if (max > 0) current.toFloat() / max.toFloat() else 1f
            if (normalized != lastNotifiedVolume) {
                lastNotifiedVolume = normalized
                try { nativeOnVolumeChange(normalized) } catch (_: Exception) {}
            }
            mainHandler.postDelayed(this, 250)
        }
    }

    // ── Browse tree ──────────────────────────────────────────────────────────

    var apiClient: OrbApiClient? = null
    private val ioExecutor = Executors.newCachedThreadPool()

    companion object {
        private const val TAG = "MediaService"
        private const val PREFS_NAME = "orb_downloads"
        private const val PREFS_KEY_METADATA = "download_metadata"
        private const val OFFLINE_DIR = "offline_audio"
        private const val OFFLINE_COVERS_DIR = "offline_covers"

        @Volatile
        var instance: MediaService? = null

        // ── JNI callbacks (Kotlin → Rust → JS) ──────────────────────────────
        @JvmStatic
        private external fun nativeOnNext()
        @JvmStatic
        private external fun nativeOnPrevious()
        @JvmStatic
        private external fun nativeOnShuffleToggle()
        @JvmStatic
        private external fun nativeOnFavoriteToggle()
        @JvmStatic
        private external fun nativeOnVolumeChange(volume: Float)
        @JvmStatic
        private external fun nativeOnPause()
        @JvmStatic
        private external fun nativeOnPlay()
        @JvmStatic
        private external fun nativeOnDownloadProgress(trackId: String, progress: Int, totalBytes: Long)
        @JvmStatic
        private external fun nativeOnABSkipBack15()
        @JvmStatic
        private external fun nativeOnABSkipForward15()
        @JvmStatic
        private external fun nativeOnABSpeedCycle()
        @JvmStatic
        private external fun nativeOnABChapterStart()
        @JvmStatic
        private external fun nativeOnPodcastSkipBack15()
        @JvmStatic
        private external fun nativeOnPodcastSkipForward30()
        @JvmStatic
        private external fun nativeOnPodcastSpeedCycle()

        init {
            System.loadLibrary("orb_lib")
        }

        // ── Static methods called from Rust via JNI ──────────────────────────

        @JvmStatic
        fun playTrack(url: String, title: String?, artist: String?, coverUrl: String?) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.handlePlay(url, title, artist, coverUrl)
            }
        }

        @JvmStatic
        fun pauseTrack() {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.player?.pause()
            }
        }

        @JvmStatic
        fun resumeTrack() {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.player?.play()
            }
        }

        @JvmStatic
        fun seekTo(positionMs: Long) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.player?.seekTo(positionMs)
            }
        }

        @JvmStatic
        fun getPosition(): Long {
            return instance?.cachedPosition ?: 0L
        }

        @JvmStatic
        fun getDuration(): Long {
            return instance?.cachedDuration ?: 0L
        }

        @JvmStatic
        fun getIsPlaying(): Boolean {
            return instance?.player?.isPlaying ?: false
        }

        @JvmStatic
        fun setShuffleState(shuffled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isShuffled = shuffled
                svc.mediaSession?.setCustomLayout(svc.buildCustomLayout())
            }
        }

        @JvmStatic
        fun setFavoriteState(favorited: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isFavorited = favorited
                svc.mediaSession?.setCustomLayout(svc.buildCustomLayout())
            }
        }

        @JvmStatic
        fun setAudiobookMode(enabled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isAudiobook = enabled
                if (enabled) svc.isPodcast = false
                svc.mediaSession?.setCustomLayout(svc.buildCustomLayout())
                // Re-set the player on the session so it re-reads available
                // commands from the ForwardingPlayer (which now conditionally
                // hides SEEK_TO_NEXT/PREVIOUS in audiobook/podcast mode), causing the
                // notification to rebuild with the correct transport controls.
                svc.wrappedPlayer?.let { wp ->
                    svc.mediaSession?.player = wp
                }
            }
        }

        @JvmStatic
        fun setPodcastMode(enabled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isPodcast = enabled
                if (enabled) svc.isAudiobook = false
                svc.mediaSession?.setCustomLayout(svc.buildCustomLayout())
                svc.wrappedPlayer?.let { wp ->
                    svc.mediaSession?.player = wp
                }
            }
        }

        @JvmStatic
        fun setPlaybackSpeed(speed: Float) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.currentSpeed = speed
                svc.player?.setPlaybackParameters(PlaybackParameters(speed))
                // Update speed icon in notification for audiobook and podcast modes
                if (svc.isAudiobook || svc.isPodcast) {
                    svc.mediaSession?.setCustomLayout(svc.buildCustomLayout())
                }
            }
        }

        @JvmStatic
        fun setApiCredentials(baseUrl: String, token: String) {
            val svc = instance ?: return
            if (svc.apiClient == null) {
                svc.apiClient = OrbApiClient(baseUrl, token)
            } else {
                svc.apiClient?.updateCredentials(baseUrl, token)
            }
        }

        /** Store the next track to play natively when the current track ends. */
        @JvmStatic
        fun setNextTrack(url: String, title: String?, artist: String?, coverUrl: String?) {
            val svc = instance ?: return
            svc.pendingNextUrl = url
            svc.pendingNextTitle = title
            svc.pendingNextArtist = artist
            svc.pendingNextCover = coverUrl
        }

        /** Clear any preloaded next track (e.g. end of queue, radio mode). */
        @JvmStatic
        fun clearNextTrack() {
            val svc = instance ?: return
            svc.pendingNextUrl = null
            svc.pendingNextTitle = null
            svc.pendingNextArtist = null
            svc.pendingNextCover = null
        }

        /**
         * Sync download metadata from the frontend.
         * JSON format: [{"trackId":"...","title":"...","artistName":"...","albumName":"...","albumId":"..."}]
         */
        @JvmStatic
        fun syncDownloads(metadataJson: String) {
            val svc = instance ?: return
            svc.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .edit()
                .putString(PREFS_KEY_METADATA, metadataJson)
                .apply()
        }

        /**
         * Download a track directly to disk to avoid OutOfMemoryError.
         * Streams content directly from URL to File.
         */
        @JvmStatic
        fun downloadTrackNative(trackId: String, url: String, authToken: String?): String {
            val svc = instance ?: throw IllegalStateException("MediaService not running")
            val dir = File(svc.filesDir, OFFLINE_DIR)
            dir.mkdirs()
            val file = File(dir, trackId)
            val tempFile = File(dir, "${trackId}.tmp")

            var connection: HttpURLConnection? = null
            try {
                val u = URL(url)
                connection = u.openConnection() as HttpURLConnection
                if (authToken != null) {
                    connection.setRequestProperty("Authorization", "Bearer $authToken")
                }
                connection.connect()

                if (connection.responseCode !in 200..299) {
                    throw Exception("Server returned ${connection.responseCode}")
                }

                val totalLength = connection.contentLength.toLong()
                val input = connection.inputStream
                val output = FileOutputStream(tempFile)

                val buffer = ByteArray(64 * 1024)
                var bytesRead: Int
                var totalRead = 0L
                var lastProgress = -1

                while (input.read(buffer).also { bytesRead = it } != -1) {
                    output.write(buffer, 0, bytesRead)
                    totalRead += bytesRead

                    if (totalLength > 0) {
                        val progress = ((totalRead * 100) / totalLength).toInt()
                        if (progress != lastProgress) {
                            lastProgress = progress
                            try { nativeOnDownloadProgress(trackId, progress, totalRead) } catch (_: Exception) {}
                        }
                    }
                }

                output.flush()
                output.close()
                input.close()

                if (tempFile.renameTo(file)) {
                    return file.absolutePath
                } else {
                    throw Exception("Failed to rename temp file")
                }
            } finally {
                connection?.disconnect()
                if (tempFile.exists()) tempFile.delete()
            }
        }

        /**
         * Save an audio file to internal storage for offline playback.
         * @deprecated Use downloadTrackNative to avoid OOM for large files.
         */
        @JvmStatic
        fun saveOfflineFile(trackId: String, data: ByteArray): String {
            val svc = instance ?: throw IllegalStateException("MediaService not running")
            val dir = File(svc.filesDir, OFFLINE_DIR)
            dir.mkdirs()
            val file = File(dir, trackId)
            file.writeBytes(data)
            return file.absolutePath
        }

        /**
         * Delete an offline audio file.
         */
        @JvmStatic
        fun deleteOfflineFile(trackId: String) {
            val svc = instance ?: return
            val file = File(File(svc.filesDir, OFFLINE_DIR), trackId)
            file.delete()
        }

        /**
         * Check if a track is available offline.
         */
        @JvmStatic
        fun hasOfflineFile(trackId: String): Boolean {
            val svc = instance ?: return false
            return File(File(svc.filesDir, OFFLINE_DIR), trackId).exists()
        }

        /**
         * Return the absolute path to an offline audio file, or null if not present.
         * Used by the frontend to build a file:// URI for ExoPlayer instead of
         * streaming over the network when the device is offline.
         */
        @JvmStatic
        fun getOfflineFilePath(trackId: String): String? {
            val svc = instance ?: return null
            val file = File(File(svc.filesDir, OFFLINE_DIR), trackId)
            return if (file.exists()) file.absolutePath else null
        }

        /**
         * Save cover art to internal storage for offline browsing.
         * Uses albumId as the filename.
         */
        @JvmStatic
        fun saveCoverArt(albumId: String, data: ByteArray) {
            val svc = instance ?: return
            val dir = File(svc.filesDir, OFFLINE_COVERS_DIR)
            dir.mkdirs()
            File(dir, albumId).writeBytes(data)
        }

        /**
         * Delete cached cover art.
         */
        @JvmStatic
        fun deleteCoverArt(albumId: String) {
            val svc = instance ?: return
            File(File(svc.filesDir, OFFLINE_COVERS_DIR), albumId).delete()
        }

        /**
         * Set system music volume. Volume is 0.0–1.0.
         */
        @JvmStatic
        fun setVolume(volume: Float) {
            val svc = instance ?: return
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val maxVol = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            val target = (volume * maxVol).toInt().coerceIn(0, maxVol)
            am.setStreamVolume(AudioManager.STREAM_MUSIC, target, 0)
        }

        /**
         * Get system music volume as 0.0–1.0.
         */
        @JvmStatic
        fun getVolume(): Float {
            val svc = instance ?: return 1.0f
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val current = am.getStreamVolume(AudioManager.STREAM_MUSIC)
            val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            return if (max > 0) current.toFloat() / max.toFloat() else 1.0f
        }

        /**
         * Apply EQ band gains to the hardware equalizer.
         * [bandsJson] is a JSON array of {frequency: Hz, gain: dB} objects
         * matching the frontend EQBand type. Pass an empty string or
         * enabled=false to disable the equalizer without changing the bands.
         */
        @JvmStatic
        fun setEQBands(enabled: Boolean, bandsJson: String) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.equalizer?.enabled = enabled
                if (enabled && bandsJson.isNotEmpty()) {
                    svc.applyEQBands(bandsJson)
                }
            }
        }

        /**
         * Configure crossfade settings. When enabled, nativeOnNext() fires
         * [secs] seconds before the track ends so the frontend can queue the
         * next track; handlePlay() then performs the volume fade.
         */
        @JvmStatic
        fun setCrossfadeSettings(enabled: Boolean, secs: Float) {
            val svc = instance ?: return
            svc.crossfadeEnabled = enabled
            svc.crossfadeSecs = secs.coerceIn(0.5f, 30f)
        }

        /**
         * Enable/disable gapless playback.
         * On native, tracks are already started promptly; this flag is stored
         * for future use (e.g. pre-buffering the next track).
         */
        @JvmStatic
        fun setGaplessEnabled(@Suppress("UNUSED_PARAMETER") enabled: Boolean) {
            // Reserved — ExoPlayer handles buffering well enough on its own.
        }

        @JvmStatic
        fun openBluetoothSettings() {
            val svc = instance ?: return
            val intent = Intent(android.provider.Settings.ACTION_BLUETOOTH_SETTINGS).apply {
                flags = Intent.FLAG_ACTIVITY_NEW_TASK
            }
            svc.startActivity(intent)
        }

        /**
         * Query the connected audio output devices and return the highest supported
         * channel count. Used to register device audio capabilities with the server
         * so the format selection engine can pick the right stream (e.g. 5.1 over eARC).
         *
         * Returns 2 (stereo) if no multi-channel output is detected.
         */
        @JvmStatic
        fun getAudioOutputMaxChannels(): Int {
            val svc = instance ?: return 2
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val outputs = am.getDevices(AudioManager.GET_DEVICES_OUTPUTS)
            // Device types that can carry multi-channel PCM audio.
            val multiChannelTypes = mutableSetOf(
                AudioDeviceInfo.TYPE_HDMI,
                AudioDeviceInfo.TYPE_HDMI_ARC,
                AudioDeviceInfo.TYPE_USB_DEVICE,
                AudioDeviceInfo.TYPE_USB_HEADSET,
            )
            // TYPE_HDMI_EARC = 29, added in API 33.
            if (Build.VERSION.SDK_INT >= 33) {
                multiChannelTypes.add(29)
            }
            var maxChannels = 2
            for (device in outputs) {
                if (device.type in multiChannelTypes) {
                    val counts = device.channelCounts
                    if (counts.isNotEmpty()) {
                        maxChannels = maxOf(maxChannels, counts.max())
                    }
                }
            }
            return maxChannels
        }
    }

    override fun onCreate() {
        super.onCreate()
        instance = this

        val notificationProvider = DefaultMediaNotificationProvider.Builder(this).build()
        notificationProvider.setSmallIcon(R.drawable.ic_notification)
        setMediaNotificationProvider(notificationProvider)

        // Disable ExoPlayer's built-in audio focus so our custom listener governs
        // all focus transitions (pause on media loss, ignore game ducking).
        val renderersFactory = DefaultRenderersFactory(this)
            .setEnableAudioFloatOutput(true)
        val exoPlayer = ExoPlayer.Builder(this, renderersFactory).build().also { p ->
            p.setAudioAttributes(
                androidx.media3.common.AudioAttributes.Builder()
                    .setUsage(androidx.media3.common.C.USAGE_MEDIA)
                    .setContentType(androidx.media3.common.C.AUDIO_CONTENT_TYPE_MUSIC)
                    .build(),
                /* handleAudioFocus= */ false
            )
        }
        player = exoPlayer

        // Start caching position/duration on main thread
        mainHandler.post(positionUpdater)

        // Auto-advance: when a track finishes, notify the frontend to play next.
        // Also listen for device volume changes (hardware buttons) for instant sync.
        exoPlayer.setDeviceVolume(exoPlayer.deviceVolume, 0) // init
        exoPlayer.addListener(object : Player.Listener {
            override fun onPlaybackStateChanged(playbackState: Int) {
                if (playbackState == Player.STATE_ENDED) {
                    if (!crossfadeTriggered) {
                        val nextUrl = pendingNextUrl
                        if (nextUrl != null) {
                            // Native auto-advance: play the preloaded next track immediately,
                            // bypassing the JS layer so playback continues when backgrounded.
                            val nextTitle = pendingNextTitle
                            val nextArtist = pendingNextArtist
                            val nextCover = pendingNextCover
                            pendingNextUrl = null
                            pendingNextTitle = null
                            pendingNextArtist = null
                            pendingNextCover = null
                            doHandlePlay(nextUrl, nextTitle, nextArtist, nextCover)
                            nativeAutoAdvanced = true
                        }
                        // Always notify JS so it can update queue state (and preload N+2).
                        try { nativeOnNext() } catch (_: Exception) {}
                    }
                    // Reset so the *next* track's end fires correctly.
                    crossfadeTriggered = false
                }
            }

            override fun onAudioSessionIdChanged(audioSessionId: Int) {
                // Re-attach the equalizer to the new audio session whenever
                // ExoPlayer opens or reopens its audio output.
                initEqualizer(audioSessionId)
            }

            override fun onIsPlayingChanged(isPlaying: Boolean) {
                try {
                    if (isPlaying) nativeOnPlay() else nativeOnPause()
                } catch (_: Exception) {}
            }

            override fun onDeviceVolumeChanged(vol: Int, muted: Boolean) {
                val am = getSystemService(Context.AUDIO_SERVICE) as AudioManager
                val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
                val normalized = if (max > 0) vol.toFloat() / max.toFloat() else 1f
                if (normalized != lastNotifiedVolume) {
                    lastNotifiedVolume = normalized
                    try { nativeOnVolumeChange(normalized) } catch (_: Exception) {}
                }
            }
        })

        // Wrap ExoPlayer so next/previous are always advertised and routed to the frontend.
        // ExoPlayer hides these buttons when there is only one item in the playlist.
        val forwardingPlayer = object : ForwardingPlayer(exoPlayer) {
            override fun getAvailableCommands(): Player.Commands {
                val base = super.getAvailableCommands().buildUpon()
                if (!isAudiobook && !isPodcast) {
                    base.add(Player.COMMAND_SEEK_TO_NEXT)
                        .add(Player.COMMAND_SEEK_TO_PREVIOUS)
                }
                return base.build()
            }

            override fun isCommandAvailable(command: Int): Boolean =
                if (command == Player.COMMAND_SEEK_TO_NEXT ||
                    command == Player.COMMAND_SEEK_TO_PREVIOUS) (!isAudiobook && !isPodcast)
                else super.isCommandAvailable(command)

            override fun seekToNext() {
                try { nativeOnNext() } catch (_: Exception) {}
            }

            override fun seekToNextMediaItem() {
                try { nativeOnNext() } catch (_: Exception) {}
            }

            override fun seekToPrevious() {
                try { nativeOnPrevious() } catch (_: Exception) {}
            }

            override fun seekToPreviousMediaItem() {
                try { nativeOnPrevious() } catch (_: Exception) {}
            }
        }
        wrappedPlayer = forwardingPlayer

        val sessionActivityIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        mediaSession = MediaSession.Builder(this, forwardingPlayer)
            .setCallback(LibraryCallback())
            .setSessionActivity(sessionActivityIntent)
            .build()

        mediaSession?.setCustomLayout(buildCustomLayout())

        // Initialize lastNotifiedVolume so the player listener can detect changes
        lastNotifiedVolume = getVolume()

        // Start the volume polling loop
        mainHandler.post(volumeChecker)
    }

    // ── MediaSession.Callback ────────────────────────────────────────────────

    private inner class LibraryCallback : MediaSession.Callback {

        override fun onConnect(
            session: MediaSession,
            controller: MediaSession.ControllerInfo
        ): MediaSession.ConnectionResult {
            val sessionCommands = MediaSession.ConnectionResult.DEFAULT_SESSION_COMMANDS.buildUpon()
                .add(SessionCommand(SHUFFLE_COMMAND, Bundle.EMPTY))
                .add(SessionCommand(FAVORITE_COMMAND, Bundle.EMPTY))
                .add(SessionCommand(AB_SKIP_BACK_15, Bundle.EMPTY))
                .add(SessionCommand(AB_SKIP_FORWARD_15, Bundle.EMPTY))
                .add(SessionCommand(AB_SPEED_CYCLE, Bundle.EMPTY))
                .add(SessionCommand(AB_CHAPTER_START, Bundle.EMPTY))
                .add(SessionCommand(POD_SKIP_BACK_15, Bundle.EMPTY))
                .add(SessionCommand(POD_SKIP_FORWARD_30, Bundle.EMPTY))
                .add(SessionCommand(POD_SPEED_CYCLE, Bundle.EMPTY))
                .build()

            return MediaSession.ConnectionResult.AcceptedResultBuilder(session)
                .setAvailableSessionCommands(sessionCommands)
                .setCustomLayout(buildCustomLayout())
                .build()
        }

        override fun onCustomCommand(
            session: MediaSession,
            controller: MediaSession.ControllerInfo,
            customCommand: SessionCommand,
            args: Bundle
        ): ListenableFuture<SessionResult> {
            when (customCommand.customAction) {
                SHUFFLE_COMMAND -> {
                    try { nativeOnShuffleToggle() } catch (_: Exception) {}
                }
                FAVORITE_COMMAND -> {
                    try { nativeOnFavoriteToggle() } catch (_: Exception) {}
                }
                AB_SKIP_BACK_15 -> {
                    try { nativeOnABSkipBack15() } catch (_: Exception) {}
                }
                AB_SKIP_FORWARD_15 -> {
                    try { nativeOnABSkipForward15() } catch (_: Exception) {}
                }
                AB_SPEED_CYCLE -> {
                    try { nativeOnABSpeedCycle() } catch (_: Exception) {}
                }
                AB_CHAPTER_START -> {
                    try { nativeOnABChapterStart() } catch (_: Exception) {}
                }
                POD_SKIP_BACK_15 -> {
                    try { nativeOnPodcastSkipBack15() } catch (_: Exception) {}
                }
                POD_SKIP_FORWARD_30 -> {
                    try { nativeOnPodcastSkipForward30() } catch (_: Exception) {}
                }
                POD_SPEED_CYCLE -> {
                    try { nativeOnPodcastSpeedCycle() } catch (_: Exception) {}
                }
            }
            return Futures.immediateFuture(SessionResult(SessionResult.RESULT_SUCCESS))
        }
    }


    // ── Offline metadata ─────────────────────────────────────────────────────

    data class DownloadMeta(
        val trackId: String,
        val title: String,
        val artistName: String?,
        val albumName: String?,
        val albumId: String?
    )

    fun getDownloadMetadata(): List<DownloadMeta> {
        return try {
            val json = getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
                .getString(PREFS_KEY_METADATA, null) ?: return emptyList()
            val arr = JSONArray(json)
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.getJSONObject(i)
                val trackId = o.optString("trackId", "")
                if (trackId.isEmpty()) return@mapNotNull null
                DownloadMeta(
                    trackId = trackId,
                    title = o.optString("title", "Unknown"),
                    artistName = optNullableString(o, "artistName"),
                    albumName = optNullableString(o, "albumName"),
                    albumId = optNullableString(o, "albumId")
                )
            }
        } catch (e: Exception) {
            Log.w(TAG, "Failed to read download metadata: ${e.message}")
            emptyList()
        }
    }

    // ── Custom layout (mode-specific notification action buttons) ───────────

    private fun buildCustomLayout(): ImmutableList<CommandButton> {
        return when {
            isAudiobook -> ImmutableList.of(
                CommandButton.Builder()
                    .setDisplayName("Skip back 15s")
                    .setIconResId(R.drawable.ic_replay_15)
                    .setSessionCommand(SessionCommand(AB_SKIP_BACK_15, Bundle.EMPTY))
                    .build(),
                CommandButton.Builder()
                    .setDisplayName("Skip forward 15s")
                    .setIconResId(R.drawable.ic_forward_15)
                    .setSessionCommand(SessionCommand(AB_SKIP_FORWARD_15, Bundle.EMPTY))
                    .build(),
                CommandButton.Builder()
                    .setDisplayName("Playback speed")
                    .setIconResId(speedIcons[currentSpeed] ?: R.drawable.ic_playback_speed_1)
                    .setSessionCommand(SessionCommand(AB_SPEED_CYCLE, Bundle.EMPTY))
                    .build(),
                CommandButton.Builder()
                    .setDisplayName("Chapter start")
                    .setIconResId(R.drawable.ic_skip_to_chapter_start)
                    .setSessionCommand(SessionCommand(AB_CHAPTER_START, Bundle.EMPTY))
                    .build()
            )
            isPodcast -> ImmutableList.of(
                CommandButton.Builder()
                    .setDisplayName("Back 15s")
                    .setIconResId(R.drawable.ic_replay_15)
                    .setSessionCommand(SessionCommand(POD_SKIP_BACK_15, Bundle.EMPTY))
                    .build(),
                CommandButton.Builder()
                    .setDisplayName("Forward 30s")
                    .setIconResId(R.drawable.ic_forward_30)
                    .setSessionCommand(SessionCommand(POD_SKIP_FORWARD_30, Bundle.EMPTY))
                    .build(),
                CommandButton.Builder()
                    .setDisplayName("Playback speed")
                    .setIconResId(speedIcons[currentSpeed] ?: R.drawable.ic_playback_speed_1)
                    .setSessionCommand(SessionCommand(POD_SPEED_CYCLE, Bundle.EMPTY))
                    .build()
            )
            else -> {
                val shuffleIcon = if (isShuffled) R.drawable.ic_shuffle else R.drawable.ic_shuffle_off
                val shuffleButton = CommandButton.Builder()
                    .setDisplayName("Shuffle")
                    .setIconResId(shuffleIcon)
                    .setSessionCommand(SessionCommand(SHUFFLE_COMMAND, Bundle.EMPTY))
                    .build()

                val favoriteIcon = if (isFavorited) R.drawable.ic_heart_filled else R.drawable.ic_heart_outline
                val favoriteButton = CommandButton.Builder()
                    .setDisplayName("Favorite")
                    .setIconResId(favoriteIcon)
                    .setSessionCommand(SessionCommand(FAVORITE_COMMAND, Bundle.EMPTY))
                    .build()

                ImmutableList.of(shuffleButton, favoriteButton)
            }
        }
    }

    // ── Audio focus ──────────────────────────────────────────────────────────

    private fun requestAudioFocus() {
        val am = getSystemService(Context.AUDIO_SERVICE) as AudioManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val req = AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
                .setAudioAttributes(
                    AudioAttributes.Builder()
                        .setUsage(AudioAttributes.USAGE_MEDIA)
                        .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
                        .build()
                )
                .setOnAudioFocusChangeListener(audioFocusListener, mainHandler)
                // Do not pause when another app requests CAN_DUCK; handle it ourselves.
                .setWillPauseWhenDucked(false)
                .build()
            audioFocusRequest = req
            am.requestAudioFocus(req)
        } else {
            @Suppress("DEPRECATION")
            am.requestAudioFocus(
                audioFocusListener,
                AudioManager.STREAM_MUSIC,
                AudioManager.AUDIOFOCUS_GAIN
            )
        }
    }

    private fun abandonAudioFocus() {
        val am = getSystemService(Context.AUDIO_SERVICE) as AudioManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            audioFocusRequest?.let { am.abandonAudioFocusRequest(it) }
            audioFocusRequest = null
        } else {
            @Suppress("DEPRECATION")
            am.abandonAudioFocus(audioFocusListener)
        }
        pausedByFocusLoss = false
    }

    // ── Equalizer ────────────────────────────────────────────────────────────

    /**
     * Attach a hardware [Equalizer] to the given ExoPlayer audio session.
     * Called whenever [onAudioSessionIdChanged] fires (i.e. on every new track).
     */
    private fun initEqualizer(audioSessionId: Int) {
        try {
            equalizer?.release()
            equalizer = Equalizer(0, audioSessionId).also { it.enabled = true }
        } catch (e: Exception) {
            Log.w(TAG, "Equalizer init failed (audioSessionId=$audioSessionId): ${e.message}")
            equalizer = null
        }
    }

    /**
     * Map frontend EQ bands (arbitrary frequencies) onto the hardware equalizer's
     * fixed bands by finding the closest center frequency in log space, then
     * apply the gain in millibels.
     */
    private fun applyEQBands(bandsJson: String) {
        val eq = equalizer ?: return
        try {
            val arr = JSONArray(bandsJson)
            val numBands = eq.numberOfBands.toInt()
            val levelRange = eq.bandLevelRange  // [minMillibels, maxMillibels] as ShortArray

            for (i in 0 until numBands) {
                // getCenterFreq returns millihertz; convert to Hz.
                val centerHz = eq.getCenterFreq(i.toShort()) / 1000.0

                // Find the frontend band whose frequency is closest (log scale).
                var closestGainDb = 0.0
                var minDist = Double.MAX_VALUE
                for (j in 0 until arr.length()) {
                    val band = arr.getJSONObject(j)
                    val freq = band.getDouble("frequency")
                    val dist = Math.abs(Math.log(freq) - Math.log(centerHz))
                    if (dist < minDist) {
                        minDist = dist
                        closestGainDb = band.getDouble("gain")
                    }
                }

                // Convert dB → millibels (100 mB = 1 dB) and clamp to device range.
                val mb = (closestGainDb * 100).toInt().toShort()
                val clamped = maxOf(levelRange[0], minOf(levelRange[1], mb))
                eq.setBandLevel(i.toShort(), clamped)
            }
        } catch (e: Exception) {
            Log.w(TAG, "applyEQBands failed: ${e.message}")
        }
    }

    // ── Playback (called from JNI) ───────────────────────────────────────────

    fun handlePlay(url: String, title: String?, artist: String?, coverUrl: String?) {
        // If native already auto-advanced to this track (WebView was backgrounded),
        // JS is just catching up — suppress the restart to avoid interrupting playback.
        if (nativeAutoAdvanced) {
            nativeAutoAdvanced = false
            return
        }
        doHandlePlay(url, title, artist, coverUrl)
    }

    /** Internal play dispatch — bypasses the nativeAutoAdvanced suppression check. */
    private fun doHandlePlay(url: String, title: String?, artist: String?, coverUrl: String?) {
        // Reset crossfade trigger for this new track.
        crossfadeTriggered = false

        if (crossfadeEnabled && crossfadeSecs > 0 && player?.isPlaying == true) {
            handlePlayWithCrossfade(url, title, artist, coverUrl)
        } else {
            handlePlayDirect(url, title, artist, coverUrl)
        }
    }

    /** Standard (non-crossfade) track switch. */
    private fun handlePlayDirect(url: String, title: String?, artist: String?, coverUrl: String?) {
        // Cancel any crossfade in progress.
        crossfadeRunnable?.let { mainHandler.removeCallbacks(it) }
        crossfadeRunnable = null
        crossfadePlayer?.stop()
        crossfadePlayer?.release()
        crossfadePlayer = null

        val mediaItem = MediaItem.Builder()
            .setUri(url)
            .setMediaMetadata(buildMetadata(title, artist))
            .build()

        requestAudioFocus()
        player?.apply {
            volume = 1f
            setMediaItem(mediaItem)
            prepare()
            play()
        }
        if (coverUrl != null) loadArtworkAsync(coverUrl)
    }

    /**
     * Crossfade track switch.
     *
     * Strategy:
     *  - A secondary ExoPlayer re-opens the OLD track at the current position
     *    and immediately plays it (fading out from 1 → 0).
     *  - The main ExoPlayer (connected to the MediaSession) switches to the NEW
     *    track at volume 0 and fades in (0 → 1).
     *  - The media notification always shows the incoming (new) track.
     *  - The secondary player is released when the fade completes.
     */
    private fun handlePlayWithCrossfade(url: String, title: String?, artist: String?, coverUrl: String?) {
        // Cancel any previous crossfade.
        crossfadeRunnable?.let { mainHandler.removeCallbacks(it) }
        crossfadeRunnable = null
        crossfadePlayer?.stop()
        crossfadePlayer?.release()
        crossfadePlayer = null

        val currentItem = player?.currentMediaItem
        val currentPos  = player?.currentPosition ?: 0L

        // Create a secondary player to continue the old track during the fade.
        if (currentItem != null) {
            val secondary = ExoPlayer.Builder(this, DefaultRenderersFactory(this).setEnableAudioFloatOutput(true)).build().also { p ->
                p.setAudioAttributes(
                    androidx.media3.common.AudioAttributes.Builder()
                        .setUsage(androidx.media3.common.C.USAGE_MEDIA)
                        .setContentType(androidx.media3.common.C.AUDIO_CONTENT_TYPE_MUSIC)
                        .build(),
                    /* handleAudioFocus= */ false  // main player already owns focus
                )
                p.volume = 1f
                p.setMediaItem(currentItem)
                p.seekTo(currentPos)
                p.prepare()
                p.play()
            }
            crossfadePlayer = secondary
        }

        // Switch main player to the new track at volume 0.
        requestAudioFocus()
        val newItem = MediaItem.Builder()
            .setUri(url)
            .setMediaMetadata(buildMetadata(title, artist))
            .build()
        player?.apply {
            volume = 0f
            setMediaItem(newItem)
            prepare()
            play()
        }
        if (coverUrl != null) loadArtworkAsync(coverUrl)

        // Step-wise volume animation over crossfadeSecs.
        val totalMs = (crossfadeSecs * 1000f).toLong().coerceAtLeast(500L)
        val steps   = 20
        val stepMs  = totalMs / steps
        var step    = 0
        val secondary = crossfadePlayer  // capture for closure

        val runnable = object : Runnable {
            override fun run() {
                step++
                val progress = step.toFloat() / steps
                player?.volume       = progress
                secondary?.volume    = 1f - progress

                if (step < steps) {
                    mainHandler.postDelayed(this, stepMs)
                } else {
                    // Fade complete — ensure volumes are exact and release the old player.
                    player?.volume = 1f
                    secondary?.stop()
                    secondary?.release()
                    if (crossfadePlayer === secondary) crossfadePlayer = null
                    crossfadeRunnable = null
                }
            }
        }
        crossfadeRunnable = runnable
        mainHandler.postDelayed(runnable, stepMs)
    }

    private fun buildMetadata(title: String?, artist: String?): MediaMetadata =
        MediaMetadata.Builder()
            .setTitle(title ?: "Unknown")
            .setArtist(artist ?: "Unknown")
            .build()

    private fun loadArtworkAsync(coverUrl: String) {
        Thread {
            try {
                val url = URL(coverUrl)
                val connection = url.openConnection()
                connection.connectTimeout = 5000
                connection.readTimeout = 5000
                val bitmap = BitmapFactory.decodeStream(connection.getInputStream())
                if (bitmap != null) {
                    Handler(Looper.getMainLooper()).post {
                        updateArtwork(bitmap)
                    }
                }
            } catch (e: Exception) {
                // Cover art loading is best-effort
            }
        }.start()
    }

    private fun updateArtwork(bitmap: Bitmap) {
        val currentItem = player?.currentMediaItem ?: return
        val updatedMetadata = currentItem.mediaMetadata.buildUpon()
            .setArtworkData(bitmapToByteArray(bitmap), MediaMetadata.PICTURE_TYPE_FRONT_COVER)
            .build()
        val updatedItem = currentItem.buildUpon()
            .setMediaMetadata(updatedMetadata)
            .build()
        player?.replaceMediaItem(0, updatedItem)
    }

    private fun bitmapToByteArray(bitmap: Bitmap): ByteArray {
        val stream = java.io.ByteArrayOutputStream()
        bitmap.compress(Bitmap.CompressFormat.JPEG, 80, stream)
        return stream.toByteArray()
    }

    fun runOnUiThread(action: Runnable) {
        mainHandler.post(action)
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? {
        return mediaSession
    }

    private fun optNullableString(o: JSONObject, key: String): String? {
        return if (o.isNull(key)) null else o.optString(key, null)
    }

    override fun onDestroy() {
        abandonAudioFocus()
        mainHandler.removeCallbacks(positionUpdater)
        mainHandler.removeCallbacks(volumeChecker)
        crossfadeRunnable?.let { mainHandler.removeCallbacks(it) }
        crossfadeRunnable = null
        crossfadePlayer?.release()
        crossfadePlayer = null
        equalizer?.release()
        equalizer = null
        mediaSession?.run {
            player.release()
            release()
        }
        mediaSession = null
        wrappedPlayer = null
        player = null
        instance = null
        super.onDestroy()
    }
}
