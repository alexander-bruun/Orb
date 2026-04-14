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
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.Timeline
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.datasource.DefaultHttpDataSource
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.CommandButton
import androidx.media3.session.DefaultMediaNotificationProvider
import androidx.media3.session.MediaLibraryService
import androidx.media3.session.MediaLibraryService.MediaLibrarySession
import androidx.media3.session.MediaSession
import androidx.media3.session.SessionCommand
import androidx.media3.session.SessionResult
import androidx.media3.session.LibraryResult
import com.google.common.collect.ImmutableList
import com.google.common.util.concurrent.Futures
import com.google.common.util.concurrent.ListenableFuture
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Callable
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

@OptIn(UnstableApi::class)
class MediaService : MediaLibraryService() {

    private var player: ExoPlayer? = null
    private var wrappedPlayer: ForwardingPlayer? = null
    private var mediaLibrarySession: MediaLibrarySession? = null
    private var httpDataSourceFactory: DefaultHttpDataSource.Factory? = null
    private var dataSourceFactory: DefaultDataSource.Factory? = null

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

    @Volatile private var currentSpeed = 1.0f
    private var isShuffled = false
    private var isFavorited = false
    @Volatile private var isAudiobook = false
    @Volatile private var isPodcast = false

    private var equalizer: Equalizer? = null
    private var crossfadePlayer: ExoPlayer? = null
    private var crossfadeRunnable: Runnable? = null
    @Volatile private var crossfadeEnabled = false
    @Volatile private var crossfadeSecs = 3f
    @Volatile private var crossfadeTriggered = false

    data class QueueTrack(
        val trackId: String,
        val title: String?,
        val artist: String?,
        val albumId: String?,
    )
    
    data class DownloadMeta(
        val trackId: String,
        val title: String,
        val artistName: String?,
        val albumName: String?,
        val albumId: String?
    )

    private val nativeQueue = mutableListOf<QueueTrack>()
    @Volatile private var nativeQueueIndex = -1
    @Volatile private var nativeRepeatMode = "off"
    @Volatile private var nativeAutoplayEnabled = false
    @Volatile private var nativeAutoAdvancedUrl: String? = null

    /** Maps trackId → albumId so we can load album queues when playing from Android Auto. */
    private val trackAlbumMap = java.util.concurrent.ConcurrentHashMap<String, String>()

    /** Last browsed track list context — used to set native queue when playing from Android Auto. */
    data class BrowseContext(val parentId: String, val tracks: List<QueueTrack>)
    @Volatile private var lastBrowseContext: BrowseContext? = null

    /** Offline-capable favorite track IDs. Persisted to SharedPreferences. */
    private val favoriteIds = java.util.concurrent.ConcurrentHashMap.newKeySet<String>()
    /**
     * Pending favorite operations that haven't been synced to the backend yet.
     * Each entry is "add:<trackId>" or "remove:<trackId>".
     */
    private val pendingFavOps = java.util.Collections.synchronizedList(mutableListOf<String>())

    @Volatile private var pausedByFocusLoss = false
    private var audioFocusRequest: AudioFocusRequest? = null

    private val audioFocusListener = AudioManager.OnAudioFocusChangeListener { focusChange ->
        when (focusChange) {
            AudioManager.AUDIOFOCUS_LOSS -> {
                pausedByFocusLoss = false
                player?.pause()
            }
            AudioManager.AUDIOFOCUS_LOSS_TRANSIENT -> {
                if (player?.isPlaying == true) {
                    pausedByFocusLoss = true
                    player?.pause()
                }
            }
            AudioManager.AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK -> {}
            AudioManager.AUDIOFOCUS_GAIN -> {
                if (pausedByFocusLoss) {
                    pausedByFocusLoss = false
                    player?.play()
                }
            }
        }
    }

    @Volatile private var cachedPosition: Long = 0L
    @Volatile private var cachedDuration: Long = 0L

    private val mainHandler = Handler(Looper.getMainLooper())
    private val positionUpdater = object : Runnable {
        override fun run() {
            player?.let {
                cachedPosition = it.currentPosition
                cachedDuration = it.duration.coerceAtLeast(0L)
                if (crossfadeEnabled && crossfadeSecs > 0 && !crossfadeTriggered && cachedDuration > 0) {
                    val remaining = cachedDuration - cachedPosition
                    val triggerMs = (crossfadeSecs * 1000f).toLong().coerceAtLeast(500L)
                    if (remaining in 1L..triggerMs) {
                        crossfadeTriggered = true
                        // Use advanceNativeQueue instead of nativeOnNext so native can
                        // auto-advance even when the WebView is suspended (app backgrounded).
                        advanceNativeQueue()
                    }
                }
            }
            mainHandler.postDelayed(this, 200)
        }
    }

    private var lastNotifiedVolume: Float = -1f
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

    var apiClient: OrbApiClient? = null
    private val pool = Executors.newCachedThreadPool()

    companion object {
        private const val TAG = "MediaService"
        private const val PREFS_NAME = "orb_downloads"
        private const val PREFS_KEY_METADATA = "download_metadata"
        private const val CREDS_PREFS = "orb_credentials"
        private const val CREDS_KEY_URL = "base_url"
        private const val CREDS_KEY_TOKEN = "token"
        private const val CREDS_KEY_REFRESH = "refresh_token"
        private const val OFFLINE_DIR = "offline_audio"
        private const val OFFLINE_COVERS_DIR = "offline_covers"
        private const val FAVS_PREFS = "orb_favorites"
        private const val FAVS_KEY_IDS = "favorite_ids"
        private const val FAVS_KEY_PENDING = "pending_ops"
        private const val ROOT_ID = "root"
        private const val CATEGORY_ALBUMS = "albums"
        private const val CATEGORY_ARTISTS = "artists"
        private const val CATEGORY_PLAYLISTS = "playlists"
        private const val CATEGORY_FAVORITES = "favorites"
        private const val CATEGORY_DOWNLOADS = "downloads"
        // Android Auto content style hints for carousel/grid rows
        private const val CONTENT_STYLE_BROWSABLE_HINT =
            "android.media.browse.CONTENT_STYLE_BROWSABLE_HINT"
        private const val CONTENT_STYLE_PLAYABLE_HINT =
            "android.media.browse.CONTENT_STYLE_PLAYABLE_HINT"
        private const val CONTENT_STYLE_GROUP_TITLE_HINT =
            "android.media.browse.CONTENT_STYLE_GROUP_TITLE_HINT"
        private const val CONTENT_STYLE_GRID = 2
        private const val CONTENT_STYLE_LIST = 1

        private const val SHUFFLE_DOWNLOADS = "shuffle_downloads"
        private const val CATEGORY_FOR_YOU = "for_you"
        private const val CATEGORY_PLAYLISTS_ROOT = "playlists_root"

        private const val PREFIX_ALBUM = "album:"
        private const val PREFIX_ARTIST = "artist:"
        private const val PREFIX_PLAYLIST = "playlist:"
        private const val PREFIX_TRACK = "track:"

        @Volatile var instance: MediaService? = null

        @JvmStatic private external fun nativeOnNext()
        @JvmStatic private external fun nativeOnPrevious()
        @JvmStatic private external fun nativeOnShuffleToggle()
        @JvmStatic private external fun nativeOnFavoriteToggle()
        @JvmStatic private external fun nativeOnVolumeChange(volume: Float)
        @JvmStatic private external fun nativeOnPause()
        @JvmStatic private external fun nativeOnPlay()
        @JvmStatic private external fun nativeOnDownloadProgress(trackId: String, progress: Int, totalBytes: Long)
        @JvmStatic private external fun nativeOnABSkipBack15()
        @JvmStatic private external fun nativeOnABSkipForward15()
        @JvmStatic private external fun nativeOnABSpeedCycle()
        @JvmStatic private external fun nativeOnABChapterStart()
        @JvmStatic private external fun nativeOnPodcastSkipBack15()
        @JvmStatic private external fun nativeOnPodcastSkipForward30()
        @JvmStatic private external fun nativeOnPodcastSpeedCycle()
        @JvmStatic private external fun nativeOnQueueAdvanced(index: Int)
        @JvmStatic private external fun nativeOnExternalPlay(trackId: String)

        init { System.loadLibrary("orb_lib") }

        @JvmStatic
        fun playTrack(url: String, title: String?, artist: String?, coverUrl: String?) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post { svc.handlePlay(url, title, artist, coverUrl) }
        }

        @JvmStatic
        fun pauseTrack() {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post { svc.player?.pause() }
        }

        @JvmStatic
        fun resumeTrack() {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post { svc.player?.play() }
        }

        @JvmStatic
        fun seekTo(positionMs: Long) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post { svc.player?.seekTo(positionMs) }
        }

        @JvmStatic fun getPosition(): Long = instance?.cachedPosition ?: 0L
        @JvmStatic fun getDuration(): Long = instance?.cachedDuration ?: 0L
        @JvmStatic fun getIsPlaying(): Boolean = instance?.player?.isPlaying ?: false

        /**
         * Return a JSON snapshot of the full playback state so the JS layer can
         * reconcile after the WebView has been frozen (app backgrounded).
         */
        @JvmStatic fun getPlaybackSnapshot(): String {
            val svc = instance
            val obj = JSONObject()
            obj.put("isPlaying", svc?.player?.isPlaying ?: false)
            obj.put("positionMs", svc?.cachedPosition ?: 0L)
            obj.put("durationMs", svc?.cachedDuration ?: 0L)
            obj.put("queueIndex", svc?.nativeQueueIndex ?: -1)
            val currentTrackId = svc?.let {
                synchronized(it.nativeQueue) {
                    it.nativeQueue.getOrNull(it.nativeQueueIndex)?.trackId
                }
            }
            obj.put("currentTrackId", currentTrackId ?: JSONObject.NULL)
            obj.put("repeatMode", svc?.nativeRepeatMode ?: "off")
            obj.put("volume", getVolume())
            obj.put("autoplayEnabled", svc?.nativeAutoplayEnabled ?: false)
            obj.put("crossfadeEnabled", svc?.crossfadeEnabled ?: false)
            obj.put("crossfadeSecs", (svc?.crossfadeSecs ?: 3f).toDouble())
            obj.put("speed", (svc?.currentSpeed ?: 1f).toDouble())
            obj.put("isAudiobook", svc?.isAudiobook ?: false)
            obj.put("isPodcast", svc?.isPodcast ?: false)
            return obj.toString()
        }

        @JvmStatic
        fun setShuffleState(shuffled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isShuffled = shuffled
                svc.mediaLibrarySession?.setCustomLayout(svc.buildCustomLayout())
            }
        }

        @JvmStatic
        fun setFavoriteState(favorited: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isFavorited = favorited
                // Keep the local favorites set in sync with the JS frontend state
                val trackId = synchronized(svc.nativeQueue) {
                    svc.nativeQueue.getOrNull(svc.nativeQueueIndex)?.trackId
                }
                if (trackId != null) {
                    if (favorited) svc.favoriteIds.add(trackId) else svc.favoriteIds.remove(trackId)
                    svc.saveFavoriteIds()
                }
                svc.mediaLibrarySession?.setCustomLayout(svc.buildCustomLayout())
            }
        }

        @JvmStatic
        fun setAudiobookMode(enabled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isAudiobook = enabled
                if (enabled) svc.isPodcast = false
                svc.mediaLibrarySession?.setCustomLayout(svc.buildCustomLayout())
                svc.wrappedPlayer?.let { wp -> svc.mediaLibrarySession?.player = wp }
            }
        }

        @JvmStatic
        fun setPodcastMode(enabled: Boolean) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.isPodcast = enabled
                if (enabled) svc.isAudiobook = false
                svc.mediaLibrarySession?.setCustomLayout(svc.buildCustomLayout())
                svc.wrappedPlayer?.let { wp -> svc.mediaLibrarySession?.player = wp }
            }
        }

        @JvmStatic
        fun setPlaybackSpeed(speed: Float) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.currentSpeed = speed
                svc.player?.setPlaybackParameters(PlaybackParameters(speed))
                if (svc.isAudiobook || svc.isPodcast) {
                    svc.mediaLibrarySession?.setCustomLayout(svc.buildCustomLayout())
                }
            }
        }

        @JvmStatic
        fun setApiCredentials(baseUrl: String, token: String, refreshToken: String) {
            val svc = instance ?: return
            val persistCallback: (String, String) -> Unit = { newToken, newRefresh ->
                svc.httpDataSourceFactory?.setDefaultRequestProperties(mapOf("Authorization" to "Bearer $newToken"))
                svc.getSharedPreferences(CREDS_PREFS, Context.MODE_PRIVATE).edit()
                    .putString(CREDS_KEY_TOKEN, newToken)
                    .putString(CREDS_KEY_REFRESH, newRefresh)
                    .apply()
            }
            if (svc.apiClient == null) {
                svc.apiClient = OrbApiClient(baseUrl, token, refreshToken).apply {
                    onTokenRefreshed = persistCallback
                }
            } else {
                svc.apiClient?.updateCredentials(baseUrl, token)
                svc.apiClient?.refreshToken = refreshToken
                svc.apiClient?.onTokenRefreshed = persistCallback
            }
            svc.httpDataSourceFactory?.setDefaultRequestProperties(mapOf("Authorization" to "Bearer $token"))
            // Persist so the browse tree works when started without the WebView
            svc.getSharedPreferences(CREDS_PREFS, Context.MODE_PRIVATE).edit()
                .putString(CREDS_KEY_URL, baseUrl)
                .putString(CREDS_KEY_TOKEN, token)
                .putString(CREDS_KEY_REFRESH, refreshToken)
                .apply()
            // Sync favorites now that the backend is reachable
            svc.refreshFavoritesFromBackend()
        }

        @JvmStatic fun setNextTrack(url: String, title: String?, artist: String?, coverUrl: String?) {}
        @JvmStatic fun clearNextTrack() {}

        @JvmStatic
        fun setPlaybackQueue(queueJson: String, currentIndex: Int, repeatMode: String) {
            val svc = instance ?: return
            try {
                val arr = JSONArray(queueJson)
                synchronized(svc.nativeQueue) {
                    svc.nativeQueue.clear()
                    for (i in 0 until arr.length()) {
                        val o = arr.getJSONObject(i)
                        svc.nativeQueue.add(QueueTrack(
                            trackId = o.getString("trackId"),
                            title = svc.optNullableString(o, "title"),
                            artist = svc.optNullableString(o, "artist"),
                            albumId = svc.optNullableString(o, "albumId"),
                        ))
                    }
                }
                svc.nativeQueueIndex = currentIndex
                svc.nativeRepeatMode = repeatMode
                svc.nativeAutoAdvancedUrl = null
            } catch (e: Exception) { Log.w(TAG, "setPlaybackQueue error: ${e.message}") }
        }

        @JvmStatic fun setNativeRepeatMode(mode: String) { instance?.nativeRepeatMode = mode }
        @JvmStatic fun setNativeAutoplay(enabled: Boolean) { instance?.nativeAutoplayEnabled = enabled }

        @JvmStatic
        fun syncDownloads(metadataJson: String) {
            val svc = instance ?: return
            svc.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE).edit()
                .putString(PREFS_KEY_METADATA, metadataJson).apply()
        }

        @JvmStatic
        fun downloadTrackNative(trackId: String, url: String, authToken: String?): String {
            val svc = instance ?: throw IllegalStateException("MediaService not running")
            val dir = File(svc.filesDir, OFFLINE_DIR).apply { mkdirs() }
            val file = File(dir, trackId)
            val tempFile = File(dir, "${trackId}.tmp")
            var connection: HttpURLConnection? = null
            try {
                connection = (URL(url).openConnection() as HttpURLConnection).apply {
                    if (authToken != null) setRequestProperty("Authorization", "Bearer $authToken")
                    connect()
                }
                if (connection.responseCode !in 200..299) throw Exception("Server error ${connection.responseCode}")
                val totalLength = connection.contentLength.toLong()
                connection.inputStream.use { input ->
                    FileOutputStream(tempFile).use { output ->
                        val buffer = ByteArray(64 * 1024)
                        var bytesRead: Int
                        var totalRead = 0L
                        while (input.read(buffer).also { bytesRead = it } != -1) {
                            output.write(buffer, 0, bytesRead)
                            totalRead += bytesRead
                            if (totalLength > 0) {
                                val progress = ((totalRead * 100) / totalLength).toInt()
                                try { nativeOnDownloadProgress(trackId, progress, totalRead) } catch (_: Exception) {}
                            }
                        }
                    }
                }
                if (tempFile.renameTo(file)) return file.absolutePath else throw Exception("Rename failed")
            } finally {
                connection?.disconnect()
                if (tempFile.exists()) tempFile.delete()
            }
        }

        @JvmStatic fun saveOfflineFile(trackId: String, data: ByteArray): String {
            val svc = instance ?: throw IllegalStateException("MediaService not running")
            val file = File(File(svc.filesDir, OFFLINE_DIR).apply { mkdirs() }, trackId)
            file.writeBytes(data)
            return file.absolutePath
        }

        @JvmStatic fun deleteOfflineFile(trackId: String) { instance?.let { File(File(it.filesDir, OFFLINE_DIR), trackId).delete() } }
        @JvmStatic fun hasOfflineFile(trackId: String): Boolean = instance?.let { File(File(it.filesDir, OFFLINE_DIR), trackId).exists() } ?: false
        @JvmStatic fun getOfflineFilePath(trackId: String): String? = instance?.let { f -> File(File(f.filesDir, OFFLINE_DIR), trackId).let { if (it.exists()) it.absolutePath else null } }
        @JvmStatic fun saveCoverArt(albumId: String, data: ByteArray) { instance?.let { f -> File(File(f.filesDir, OFFLINE_COVERS_DIR).apply { mkdirs() }, albumId).writeBytes(data) } }
        @JvmStatic fun deleteCoverArt(albumId: String) { instance?.let { File(File(it.filesDir, OFFLINE_COVERS_DIR), albumId).delete() } }

        @JvmStatic
        fun setVolume(volume: Float) {
            val svc = instance ?: return
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val maxVol = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            am.setStreamVolume(AudioManager.STREAM_MUSIC, (volume * maxVol).toInt().coerceIn(0, maxVol), 0)
        }

        @JvmStatic
        fun getVolume(): Float {
            val svc = instance ?: return 1.0f
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val max = am.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            return if (max > 0) am.getStreamVolume(AudioManager.STREAM_MUSIC).toFloat() / max.toFloat() else 1.0f
        }

        @JvmStatic
        fun setEQBands(enabled: Boolean, bandsJson: String) {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.equalizer?.enabled = enabled
                if (enabled && bandsJson.isNotEmpty()) svc.applyEQBands(bandsJson)
            }
        }

        @JvmStatic
        fun clearApiCredentials() {
            val svc = instance ?: return
            Handler(Looper.getMainLooper()).post {
                svc.apiClient = null
                svc.httpDataSourceFactory?.setDefaultRequestProperties(emptyMap())
                svc.getSharedPreferences(CREDS_PREFS, Context.MODE_PRIVATE).edit().clear().apply()
                // Stop any active playback since user logged out
                svc.player?.stop()
                svc.player?.clearMediaItems()
                synchronized(svc.nativeQueue) { svc.nativeQueue.clear() }
                svc.nativeQueueIndex = -1
                svc.lastBrowseContext = null
                svc.trackAlbumMap.clear()
                Log.d(TAG, "API credentials cleared — user logged out")
            }
        }

        @JvmStatic fun setCrossfadeSettings(enabled: Boolean, secs: Float) { instance?.apply { crossfadeEnabled = enabled; crossfadeSecs = secs.coerceIn(0.5f, 30f) } }
        @JvmStatic fun setGaplessEnabled(enabled: Boolean) {}

        @JvmStatic
        fun openBluetoothSettings() {
            instance?.startActivity(Intent(android.provider.Settings.ACTION_BLUETOOTH_SETTINGS).apply { flags = Intent.FLAG_ACTIVITY_NEW_TASK })
        }

        @JvmStatic
        fun getAudioOutputMaxChannels(): Int {
            val svc = instance ?: return 2
            val am = svc.getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val outputs = am.getDevices(AudioManager.GET_DEVICES_OUTPUTS)
            val multiChannelTypes = mutableSetOf(AudioDeviceInfo.TYPE_HDMI, AudioDeviceInfo.TYPE_HDMI_ARC, AudioDeviceInfo.TYPE_USB_DEVICE, AudioDeviceInfo.TYPE_USB_HEADSET)
            if (Build.VERSION.SDK_INT >= 33) multiChannelTypes.add(29)
            var maxChannels = 2
            for (device in outputs) {
                if (device.type in multiChannelTypes) {
                    val counts = device.channelCounts
                    if (counts.isNotEmpty()) maxChannels = maxOf(maxChannels, counts.max())
                }
            }
            return maxChannels
        }
    }

    override fun onCreate() {
        super.onCreate()
        Log.d(TAG, "MediaService.onCreate() - STARTING")
        instance = this

        val notificationProvider = DefaultMediaNotificationProvider.Builder(this).build()
        notificationProvider.setSmallIcon(R.drawable.ic_notification)
        setMediaNotificationProvider(notificationProvider)

        val renderersFactory = DefaultRenderersFactory(this).setEnableAudioFloatOutput(true)
        val httpDataSourceFactory = DefaultHttpDataSource.Factory().setAllowCrossProtocolRedirects(true)
        this.httpDataSourceFactory = httpDataSourceFactory
        val dataSourceFactory = DefaultDataSource.Factory(this, httpDataSourceFactory)
        this.dataSourceFactory = dataSourceFactory

        val exoPlayer = ExoPlayer.Builder(this, renderersFactory)
            .setMediaSourceFactory(DefaultMediaSourceFactory(this).setDataSourceFactory(dataSourceFactory))
            .build().also { p ->
                p.setAudioAttributes(androidx.media3.common.AudioAttributes.Builder()
                    .setUsage(androidx.media3.common.C.USAGE_MEDIA)
                    .setContentType(androidx.media3.common.C.AUDIO_CONTENT_TYPE_MUSIC)
                    .build(), false)
            }
        player = exoPlayer
        mainHandler.post(positionUpdater)

        exoPlayer.addListener(object : Player.Listener {
            override fun onPlaybackStateChanged(playbackState: Int) {
                mediaLibrarySession?.setCustomLayout(buildCustomLayout())
                mediaLibrarySession?.player = wrappedPlayer ?: exoPlayer
                
                if (playbackState == Player.STATE_ENDED) {
                    advanceNativeQueue()
                }
            }
            override fun onMediaItemTransition(mediaItem: MediaItem?, reason: Int) {
                mediaLibrarySession?.player = wrappedPlayer ?: exoPlayer
                // Update favorite button to reflect the new track's state
                val trackId = synchronized(nativeQueue) {
                    nativeQueue.getOrNull(nativeQueueIndex)?.trackId
                }
                if (trackId != null) updateFavoriteStateForTrack(trackId)
            }
            override fun onAudioSessionIdChanged(audioSessionId: Int) { initEqualizer(audioSessionId) }
            override fun onIsPlayingChanged(isPlaying: Boolean) {
                try { if (isPlaying) nativeOnPlay() else nativeOnPause() } catch (_: Exception) {}
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

        wrappedPlayer = object : ForwardingPlayer(exoPlayer) {
            override fun getAvailableCommands(): Player.Commands = super.getAvailableCommands().buildUpon()
                .add(Player.COMMAND_PLAY_PAUSE).add(Player.COMMAND_SEEK_TO_NEXT).add(Player.COMMAND_SEEK_TO_PREVIOUS).build()
            override fun isCommandAvailable(command: Int): Boolean = when (command) {
                Player.COMMAND_PLAY_PAUSE, Player.COMMAND_SEEK_TO_NEXT, Player.COMMAND_SEEK_TO_PREVIOUS -> true
                else -> super.isCommandAvailable(command)
            }
            override fun seekToNext() {
                if (synchronized(nativeQueue) { nativeQueue.isNotEmpty() }) advanceNativeQueue()
                else try { nativeOnNext() } catch (_: Exception) {}
            }
            override fun seekToNextMediaItem() {
                if (synchronized(nativeQueue) { nativeQueue.isNotEmpty() }) advanceNativeQueue()
                else try { nativeOnNext() } catch (_: Exception) {}
            }
            override fun seekToPrevious() { try { nativeOnPrevious() } catch (_: Exception) {} }
            override fun seekToPreviousMediaItem() { try { nativeOnPrevious() } catch (_: Exception) {} }
            override fun play() {
                // Always request audio focus — critical for Android Auto which
                // calls play() directly via the session without going through handlePlay().
                requestAudioFocus()
                super.play()
            }
            override fun pause() { super.pause() }

            // Expose the full nativeQueue to Android Auto's queue view.
            // ExoPlayer only holds the current single item; overriding these
            // lets the MediaSession build a multi-item timeline for display.
            override fun getCurrentMediaItemIndex(): Int =
                nativeQueueIndex.coerceAtLeast(0)

            override fun getCurrentTimeline(): Timeline {
                val snapshot: List<QueueTrack>
                val idx: Int
                synchronized(nativeQueue) {
                    snapshot = nativeQueue.toList()
                    idx = nativeQueueIndex
                }
                if (snapshot.isEmpty()) return super.getCurrentTimeline()
                val currentMi = super.getCurrentMediaItem()
                return object : Timeline() {
                    override fun getWindowCount(): Int = snapshot.size
                    override fun getWindow(windowIndex: Int, window: Window, defaultPositionProjectionUs: Long): Window {
                        val track = snapshot[windowIndex]
                        val mi = if (windowIndex == idx && currentMi != null) currentMi
                                 else buildQueueMediaItem(track)
                        return window.set(
                            windowIndex,
                            mi,
                            null,
                            C.TIME_UNSET,
                            C.TIME_UNSET,
                            C.TIME_UNSET,
                            true,
                            false,
                            null,
                            0L,
                            C.TIME_UNSET,
                            windowIndex,
                            windowIndex,
                            0L
                        )
                    }
                    override fun getPeriodCount(): Int = snapshot.size
                    override fun getPeriod(periodIndex: Int, period: Period, setIds: Boolean): Period =
                        period.set(
                            if (setIds) periodIndex else null,
                            if (setIds) periodIndex.toLong() else null,
                            periodIndex,
                            C.TIME_UNSET,
                            0L
                        )
                    override fun getIndexOfPeriod(uid: Any): Int =
                        (uid as? Long)?.toInt() ?: (uid as? Int) ?: C.INDEX_UNSET
                    override fun getUidOfPeriod(periodIndex: Int): Any = periodIndex.toLong()
                }
            }
        }

        val sessionActivityIntent = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

        mediaLibrarySession = MediaLibrarySession.Builder(this, wrappedPlayer!!, LibraryCallback())
            .setSessionActivity(sessionActivityIntent)
            .setBitmapLoader(AuthBitmapLoader())
            .build().apply {
                player = wrappedPlayer!!
                setCustomLayout(buildCustomLayout())
            }

        // Restore API credentials so browse tree works without the WebView
        if (apiClient == null) {
            val creds = getSharedPreferences(CREDS_PREFS, Context.MODE_PRIVATE)
            val savedUrl = creds.getString(CREDS_KEY_URL, null)
            val savedToken = creds.getString(CREDS_KEY_TOKEN, null)
            val savedRefresh = creds.getString(CREDS_KEY_REFRESH, null)
            if (!savedUrl.isNullOrEmpty() && !savedToken.isNullOrEmpty()) {
                apiClient = OrbApiClient(savedUrl, savedToken, savedRefresh ?: "").apply {
                    onTokenRefreshed = { newToken, newRefresh ->
                        // Persist refreshed credentials back
                        httpDataSourceFactory?.setDefaultRequestProperties(mapOf("Authorization" to "Bearer $newToken"))
                        getSharedPreferences(CREDS_PREFS, Context.MODE_PRIVATE).edit()
                            .putString(CREDS_KEY_TOKEN, newToken)
                            .putString(CREDS_KEY_REFRESH, newRefresh)
                            .apply()
                    }
                }
                httpDataSourceFactory?.setDefaultRequestProperties(mapOf("Authorization" to "Bearer $savedToken"))
                Log.d(TAG, "Restored API credentials from SharedPreferences")
                // Pull favorite state from the backend now that we have credentials
                refreshFavoritesFromBackend()
            }
        }

        // Load offline favorites cache (available immediately, even without backend)
        loadFavoritesFromPrefs()

        lastNotifiedVolume = getVolume()
        mainHandler.post(volumeChecker)
        Log.d(TAG, "MediaService.onCreate() - COMPLETE")
    }

    private inner class LibraryCallback : MediaLibrarySession.Callback {
        override fun onConnect(session: MediaSession, controller: MediaSession.ControllerInfo): MediaSession.ConnectionResult {
            Log.d(TAG, "onConnect from ${controller.packageName}")

            // Allow our own app to connect unconditionally (the WebView needs the
            // session even before the user has logged in).
            val isSelf = controller.packageName == packageName

            // Reject external controllers (Android Auto, etc.) when not authenticated.
            if (!isSelf && apiClient == null) {
                Log.w(TAG, "Rejecting connection from ${controller.packageName} — no credentials")
                return MediaSession.ConnectionResult.reject()
            }

            val sessionCommands = MediaSession.ConnectionResult.DEFAULT_SESSION_AND_LIBRARY_COMMANDS.buildUpon()
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

        override fun onPlaybackResumption(
            session: MediaSession, 
            controller: MediaSession.ControllerInfo
        ): ListenableFuture<MediaSession.MediaItemsWithStartPosition> {
            Log.d(TAG, "onPlaybackResumption triggered for ${controller.packageName}")
            
            val currentItem = player?.currentMediaItem
            val currentIndex = player?.currentMediaItemIndex ?: 0
            val currentPos = player?.currentPosition ?: 0L

            val result = if (currentItem != null) {
                MediaSession.MediaItemsWithStartPosition(
                    listOf(currentItem),
                    currentIndex,
                    currentPos
                )
            } else {
                MediaSession.MediaItemsWithStartPosition(
                    emptyList(),
                    0,
                    0L
                )
            }
            return Futures.immediateFuture(result)
        }

        override fun onGetLibraryRoot(session: MediaLibrarySession, browser: MediaSession.ControllerInfo, params: MediaLibraryService.LibraryParams?): ListenableFuture<LibraryResult<MediaItem>> {
            Log.d(TAG, "onGetLibraryRoot from ${browser.packageName}")
            val rootExtras = Bundle().apply {
                putInt(CONTENT_STYLE_BROWSABLE_HINT, CONTENT_STYLE_GRID)
                putInt(CONTENT_STYLE_PLAYABLE_HINT, CONTENT_STYLE_LIST)
            }
            val root = MediaItem.Builder().setMediaId(ROOT_ID).setMediaMetadata(MediaMetadata.Builder()
                .setTitle("Orb").setIsBrowsable(true).setIsPlayable(false)
                .setExtras(rootExtras).build()).build()
            val resultParams = MediaLibraryService.LibraryParams.Builder()
                .setExtras(rootExtras).build()
            return Futures.immediateFuture(LibraryResult.ofItem(root, resultParams))
        }

        override fun onGetChildren(session: MediaLibrarySession, browser: MediaSession.ControllerInfo, parentId: String, page: Int, pageSize: Int, params: MediaLibraryService.LibraryParams?): ListenableFuture<LibraryResult<ImmutableList<MediaItem>>> {
            Log.d(TAG, "onGetChildren parentId=$parentId from ${browser.packageName}")
            // Root — top-level tabs for Android Auto
            if (parentId == ROOT_ID) {
                val settableRoot = com.google.common.util.concurrent.SettableFuture.create<LibraryResult<ImmutableList<MediaItem>>>()
                pool.execute {
                    val api = apiClient
                    val online = api != null && api.isReachable()
                    val items = if (online) {
                        listOf(
                            browsableItem(CATEGORY_FOR_YOU, "For You"),
                            browsableItem(CATEGORY_PLAYLISTS_ROOT, "Playlists")
                        )
                    } else {
                        listOf(browsableItem(CATEGORY_DOWNLOADS, "Downloads"))
                    }
                    settableRoot.set(LibraryResult.ofItemList(ImmutableList.copyOf(items), null))
                }
                return settableRoot
            }

            // For You tab — recently played + made-for-you albums
            if (parentId == CATEGORY_FOR_YOU) {
                val settable = com.google.common.util.concurrent.SettableFuture.create<LibraryResult<ImmutableList<MediaItem>>>()
                pool.execute {
                    val api = apiClient
                    if (api == null) { settable.set(LibraryResult.ofItemList(ImmutableList.of(), null)); return@execute }
                    val items = mutableListOf<MediaItem>()
                    try {
                        val recent = api.recentlyPlayedAlbums().take(10)
                        items.addAll(recent.map { albumToMediaItem(it, api, "Recently Played") })
                    } catch (_: Exception) {}
                    try {
                        val tracks = api.mostPlayedTracks(50)
                        val seen = mutableSetOf<String>()
                        val recAlbums = mutableListOf<OrbApiClient.BrowseAlbum>()
                        for (t in tracks) {
                            val albumId = t.albumId ?: continue
                            if (seen.add(albumId)) {
                                recAlbums.add(OrbApiClient.BrowseAlbum(albumId, t.albumName ?: "Unknown", t.artistName, null))
                                if (recAlbums.size >= 10) break
                            }
                        }
                        items.addAll(recAlbums.map { albumToMediaItem(it, api, "Made For You") })
                    } catch (_: Exception) {}
                    settable.set(LibraryResult.ofItemList(ImmutableList.copyOf(items), null))
                }
                return settable
            }

            // Playlists tab
            if (parentId == CATEGORY_PLAYLISTS_ROOT) {
                val settable = com.google.common.util.concurrent.SettableFuture.create<LibraryResult<ImmutableList<MediaItem>>>()
                pool.execute {
                    val api = apiClient
                    if (api == null) { settable.set(LibraryResult.ofItemList(ImmutableList.of(), null)); return@execute }
                    val playlists = try { api.playlists() } catch (_: Exception) { emptyList() }
                    // Pre-fetch playlist cover art in parallel — Android Auto doesn't
                    // load artworkUri for browse items, so we embed the bitmap directly.
                    val coverFutures = playlists.associate { pl ->
                        pl.id to pool.submit(Callable { fetchScaledCoverBytes(api.playlistCoverUrl(pl.id), api) })
                    }
                    val coverBytes = coverFutures.mapValues { (_, f) ->
                        try { f.get(5, TimeUnit.SECONDS) } catch (_: Exception) { null }
                    }
                    settable.set(LibraryResult.ofItemList(ImmutableList.copyOf(
                        playlists.map { playlistToMediaItem(it, api, artworkBytes = coverBytes[it.id]) }
                    ), null))
                }
                return settable
            }

            // Downloads — works offline, no API needed
            if (parentId == CATEGORY_DOWNLOADS) {
                val svc = this@MediaService
                val authority = "${svc.packageName}.fileprovider"
                val downloads = svc.getDownloadMetadata()
                val items = mutableListOf<MediaItem>()
                val queueTracks = mutableListOf<QueueTrack>()

                // First item: Shuffle All
                if (downloads.any { dl -> File(File(svc.filesDir, OFFLINE_DIR), dl.trackId).exists() }) {
                    items.add(MediaItem.Builder()
                        .setMediaId(SHUFFLE_DOWNLOADS)
                        .setMediaMetadata(MediaMetadata.Builder()
                            .setTitle("\u25B6 Shuffle All Downloads")
                            .setIsBrowsable(false)
                            .setIsPlayable(true)
                            .build())
                        .build())
                }

                for (dl in downloads) {
                    val audioFile = File(File(svc.filesDir, OFFLINE_DIR), dl.trackId)
                    if (!audioFile.exists()) continue
                    val artUri = dl.albumId?.let { aId ->
                        val coverFile = File(File(svc.filesDir, OFFLINE_COVERS_DIR), aId)
                        if (coverFile.exists()) {
                            try {
                                androidx.core.content.FileProvider.getUriForFile(svc, authority, coverFile)
                            } catch (_: Exception) { null }
                        } else null
                    }
                    trackAlbumMap[dl.trackId] = dl.albumId ?: ""
                    items.add(MediaItem.Builder()
                        .setMediaId("$PREFIX_TRACK${dl.trackId}")
                        .setMediaMetadata(MediaMetadata.Builder()
                            .setTitle(dl.title)
                            .setArtist(dl.artistName)
                            .setAlbumTitle(dl.albumName)
                            .setArtworkUri(artUri)
                            .setIsBrowsable(false)
                            .setIsPlayable(true)
                            .build())
                        .setUri("file://${audioFile.absolutePath}")
                        .build())
                    queueTracks.add(QueueTrack(trackId = dl.trackId, title = dl.title, artist = dl.artistName, albumId = dl.albumId))
                }
                if (queueTracks.isNotEmpty()) {
                    lastBrowseContext = BrowseContext(parentId, queueTracks)
                }
                return Futures.immediateFuture(LibraryResult.ofItemList(ImmutableList.copyOf(items), null))
            }

            // Everything else hits the API — resolve on the IO pool
            val settable = com.google.common.util.concurrent.SettableFuture.create<LibraryResult<ImmutableList<MediaItem>>>()
            pool.execute {
                val api = apiClient
                if (api == null) {
                    settable.set(LibraryResult.ofItemList(ImmutableList.of(), null))
                    return@execute
                }
                try {
                    val items: List<MediaItem> = when {
                        parentId == CATEGORY_ALBUMS -> api.albums(100, 0).map { albumToMediaItem(it, api) }
                        parentId == CATEGORY_ARTISTS -> api.artists(100, 0).map { artistToMediaItem(it) }
                        parentId == CATEGORY_PLAYLISTS -> {
                            val pls = api.playlists()
                            val covers = pls.associate { pl ->
                                pl.id to pool.submit(Callable { fetchScaledCoverBytes(api.playlistCoverUrl(pl.id), api) })
                            }.mapValues { (_, f) -> try { f.get(5, TimeUnit.SECONDS) } catch (_: Exception) { null } }
                            pls.map { playlistToMediaItem(it, api, artworkBytes = covers[it.id]) }
                        }
                        parentId == CATEGORY_FAVORITES -> api.favorites().map { trackToMediaItem(it, api) }
                        parentId.startsWith(PREFIX_ALBUM) -> {
                            val albumId = parentId.removePrefix(PREFIX_ALBUM)
                            val detail = api.albumDetail(albumId)
                            detail?.tracks?.forEach { t -> trackAlbumMap[t.id] = albumId }
                            detail?.tracks?.map { trackToMediaItem(it, api) } ?: emptyList()
                        }
                        parentId.startsWith(PREFIX_ARTIST) -> {
                            val artistId = parentId.removePrefix(PREFIX_ARTIST)
                            api.artistAlbums(artistId).map { albumToMediaItem(it, api) }
                        }
                        parentId.startsWith(PREFIX_PLAYLIST) -> {
                            val playlistId = parentId.removePrefix(PREFIX_PLAYLIST)
                            api.playlistDetail(playlistId)?.tracks?.map { trackToMediaItem(it, api) } ?: emptyList()
                        }
                        else -> emptyList()
                    }
                    // Cache track lists as browse context so the full list becomes
                    // the native queue when a track from this list is played.
                    val playableItems = items.filter { it.mediaMetadata.isPlayable == true }
                    if (playableItems.isNotEmpty()) {
                        lastBrowseContext = BrowseContext(parentId, playableItems.map { mi ->
                            val tid = mi.mediaId.removePrefix(PREFIX_TRACK)
                            QueueTrack(
                                trackId = tid,
                                title = mi.mediaMetadata.title?.toString(),
                                artist = mi.mediaMetadata.artist?.toString(),
                                albumId = trackAlbumMap[tid]
                            )
                        })
                    }
                    settable.set(LibraryResult.ofItemList(ImmutableList.copyOf(items), null))
                } catch (e: Exception) {
                    Log.w(TAG, "onGetChildren($parentId) failed: ${e.message}")
                    settable.set(LibraryResult.ofItemList(ImmutableList.of(), null))
                }
            }
            return settable
        }

        override fun onGetItem(session: MediaLibrarySession, browser: MediaSession.ControllerInfo, mediaId: String): ListenableFuture<LibraryResult<MediaItem>> {
            // For track IDs, the car system requests the item to show Now Playing metadata.
            // Return full metadata (title, artist, artwork) so the controls page shows cover art.
            if (mediaId.startsWith(PREFIX_TRACK)) {
                val trackId = mediaId.removePrefix(PREFIX_TRACK)

                // First check if the player already has this item with full metadata
                val currentItem = player?.currentMediaItem
                if (currentItem != null && currentItem.mediaId == mediaId && currentItem.mediaMetadata.artworkUri != null) {
                    return Futures.immediateFuture(LibraryResult.ofItem(currentItem, null))
                }

                // Look up metadata from the browse context cache
                val browseTrack = lastBrowseContext?.tracks?.firstOrNull { it.trackId == trackId }
                val albumId = browseTrack?.albumId ?: trackAlbumMap[trackId]
                // Use buildCoverUrlForAlbum so the offline cover file is checked first,
                // returning a file:// URI instead of a network URL when available.
                // This prevents the Now Playing artwork from going blank when offline.
                val artUri = buildCoverUrlForAlbum(albumId)?.let { Uri.parse(it) }

                val item = MediaItem.Builder().setMediaId(mediaId)
                    .setMediaMetadata(MediaMetadata.Builder()
                        .setTitle(browseTrack?.title)
                        .setArtist(browseTrack?.artist)
                        .setArtworkUri(artUri)
                        .setIsPlayable(true)
                        .setIsBrowsable(false)
                        .build()).build()
                return Futures.immediateFuture(LibraryResult.ofItem(item, null))
            }
            val item = MediaItem.Builder().setMediaId(mediaId)
                .setMediaMetadata(MediaMetadata.Builder().setIsBrowsable(true).setIsPlayable(false).build()).build()
            return Futures.immediateFuture(LibraryResult.ofItem(item, null))
        }

        private fun browsableItem(id: String, title: String, groupTitle: String? = null): MediaItem {
            val extras = groupTitle?.let { Bundle().apply { putString(CONTENT_STYLE_GROUP_TITLE_HINT, it) } }
            return MediaItem.Builder()
                .setMediaId(id)
                .setMediaMetadata(MediaMetadata.Builder()
                    .setTitle(title)
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .apply { if (extras != null) setExtras(extras) }
                    .build())
                .build()
        }

        private fun albumToMediaItem(album: OrbApiClient.BrowseAlbum, api: OrbApiClient, groupTitle: String? = null): MediaItem {
            val artUri = Uri.parse(api.coverUrl(album.id))
            val extras = groupTitle?.let { Bundle().apply { putString(CONTENT_STYLE_GROUP_TITLE_HINT, it) } }
            return MediaItem.Builder()
                .setMediaId("$PREFIX_ALBUM${album.id}")
                .setMediaMetadata(MediaMetadata.Builder()
                    .setTitle(album.title)
                    .setArtist(album.artistName)
                    .setArtworkUri(artUri)
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .apply { if (extras != null) setExtras(extras) }
                    .build())
                .build()
        }

        private fun artistToMediaItem(artist: OrbApiClient.BrowseArtist): MediaItem {
            return MediaItem.Builder()
                .setMediaId("$PREFIX_ARTIST${artist.id}")
                .setMediaMetadata(MediaMetadata.Builder()
                    .setTitle(artist.name)
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .build())
                .build()
        }

        private fun playlistToMediaItem(playlist: OrbApiClient.BrowsePlaylist, api: OrbApiClient, groupTitle: String? = null, artworkBytes: ByteArray? = null): MediaItem {
            val artUri = Uri.parse(api.playlistCoverUrl(playlist.id))
            val extras = groupTitle?.let { Bundle().apply { putString(CONTENT_STYLE_GROUP_TITLE_HINT, it) } }
            return MediaItem.Builder()
                .setMediaId("$PREFIX_PLAYLIST${playlist.id}")
                .setMediaMetadata(MediaMetadata.Builder()
                    .setTitle(playlist.name)
                    .setArtist(playlist.description)
                    .setArtworkUri(artUri)
                    .apply { if (artworkBytes != null) setArtworkData(artworkBytes, MediaMetadata.PICTURE_TYPE_FRONT_COVER) }
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .apply { if (extras != null) setExtras(extras) }
                    .build())
                .build()
        }

        private fun trackToMediaItem(track: OrbApiClient.BrowseTrack, api: OrbApiClient): MediaItem {
            // Cache trackId → albumId for queue loading when playing from Android Auto
            track.albumId?.let { trackAlbumMap[track.id] = it }
            val artUri = track.albumId?.let { Uri.parse(api.coverUrl(it)) }
            return MediaItem.Builder()
                .setMediaId("$PREFIX_TRACK${track.id}")
                .setMediaMetadata(MediaMetadata.Builder()
                    .setTitle(track.title)
                    .setArtist(track.artistName)
                    .setAlbumTitle(track.albumName)
                    .setArtworkUri(artUri)
                    .setIsBrowsable(false)
                    .setIsPlayable(true)
                    .setDurationMs(track.durationMs)
                    .build())
                .setUri(api.streamUrl(track.id))
                .build()
        }

        override fun onAddMediaItems(
            mediaSession: MediaSession,
            controller: MediaSession.ControllerInfo,
            mediaItems: MutableList<MediaItem>
        ): ListenableFuture<MutableList<MediaItem>> {
            val api = apiClient
            val resolved = mutableListOf<MediaItem>()

            for (item in mediaItems) {
                val mediaId = item.mediaId

                // Shuffle Downloads: build a shuffled queue from all offline tracks
                if (mediaId == SHUFFLE_DOWNLOADS) {
                    val allDownloads = this@MediaService.getDownloadMetadata()
                    val available = allDownloads.filter { dl ->
                        File(File(this@MediaService.filesDir, OFFLINE_DIR), dl.trackId).exists()
                    }.shuffled()
                    if (available.isEmpty()) continue

                    // Set native queue with all shuffled tracks
                    val shuffledQueue = available.map { dl ->
                        QueueTrack(trackId = dl.trackId, title = dl.title, artist = dl.artistName, albumId = dl.albumId)
                    }
                    mainHandler.post {
                        synchronized(nativeQueue) {
                            nativeQueue.clear()
                            nativeQueue.addAll(shuffledQueue)
                        }
                        nativeQueueIndex = 0
                        nativeAutoAdvancedUrl = null
                    }

                    // Resolve the first track as the item to play
                    val first = available[0]
                    val firstPath = getOfflineFilePath(first.trackId) ?: continue
                    val coverUrl = buildCoverUrlForAlbum(first.albumId)
                    resolved.add(MediaItem.Builder()
                        .setMediaId("$PREFIX_TRACK${first.trackId}")
                        .setUri("file://$firstPath")
                        .setMediaMetadata(MediaMetadata.Builder()
                            .setTitle(first.title)
                            .setArtist(first.artistName)
                            .setAlbumTitle(first.albumName)
                            .setArtworkUri(coverUrl?.let { Uri.parse(it) })
                            .setIsPlayable(true)
                            .setIsBrowsable(false)
                            .build())
                        .build())
                    mainHandler.post { try { nativeOnExternalPlay(first.trackId) } catch (_: Exception) {} }
                    continue
                }

                if (mediaId.startsWith(PREFIX_TRACK)) {
                    val trackId = mediaId.removePrefix(PREFIX_TRACK)

                    // Use offline file if available, otherwise stream from server
                    val offlinePath = getOfflineFilePath(trackId)
                    val streamUri = if (offlinePath != null) {
                        "file://$offlinePath"
                    } else {
                        api?.streamUrl(trackId) ?: item.requestMetadata.mediaUri?.toString() ?: ""
                    }

                    // Notify the JS frontend about this external play
                    mainHandler.post { try { nativeOnExternalPlay(trackId) } catch (_: Exception) {} }

                    // Set the last browsed track list as the native queue so the
                    // rest of the album/playlist plays after this track.
                    val context = lastBrowseContext
                    if (context != null && context.tracks.any { it.trackId == trackId }) {
                        val index = context.tracks.indexOfFirst { it.trackId == trackId }.coerceAtLeast(0)
                        mainHandler.post {
                            synchronized(nativeQueue) {
                                nativeQueue.clear()
                                nativeQueue.addAll(context.tracks)
                            }
                            nativeQueueIndex = index
                            nativeAutoAdvancedUrl = null
                        }
                    } else {
                        // Fallback: try to load the album tracks as queue
                        val albumId = trackAlbumMap[trackId]
                        if (albumId != null && albumId.isNotEmpty()) {
                            pool.execute { loadAlbumQueueForTrack(trackId, albumId) }
                        }
                    }

                    // Reconstruct full metadata (title, artist, artwork) so artwork
                    // is always present — Android Auto sends minimal MediaItems with
                    // only the mediaId, so the browse-tree metadata is lost.
                    val existingMeta = item.mediaMetadata
                    val albumId = trackAlbumMap[trackId]
                    val artUri = existingMeta.artworkUri
                        ?: albumId?.let { api?.let { a -> Uri.parse(a.coverUrl(it)) } }
                    val browseTrack = context?.tracks?.firstOrNull { it.trackId == trackId }

                    val rebuiltMeta = MediaMetadata.Builder()
                        .setTitle(existingMeta.title ?: browseTrack?.title ?: "Unknown")
                        .setArtist(existingMeta.artist ?: browseTrack?.artist ?: "Unknown")
                        .setArtworkUri(artUri)
                        .setIsPlayable(true)
                        .setIsBrowsable(false)
                        .build()

                    resolved.add(item.buildUpon()
                        .setUri(streamUri)
                        .setMediaId(mediaId)
                        .setMediaMetadata(rebuiltMeta)
                        .build())
                } else {
                    resolved.add(item)
                }
            }
            return Futures.immediateFuture(resolved)
        }

        override fun onCustomCommand(session: MediaSession, controller: MediaSession.ControllerInfo, customCommand: SessionCommand, args: Bundle): ListenableFuture<SessionResult> {
            when (customCommand.customAction) {
                SHUFFLE_COMMAND -> try { nativeOnShuffleToggle() } catch (_: Exception) {}
                FAVORITE_COMMAND -> this@MediaService.toggleFavoriteForCurrentTrack()
                AB_SKIP_BACK_15 -> try { nativeOnABSkipBack15() } catch (_: Exception) {}
                AB_SKIP_FORWARD_15 -> try { nativeOnABSkipForward15() } catch (_: Exception) {}
                AB_SPEED_CYCLE -> try { nativeOnABSpeedCycle() } catch (_: Exception) {}
                AB_CHAPTER_START -> try { nativeOnABChapterStart() } catch (_: Exception) {}
                POD_SKIP_BACK_15 -> try { nativeOnPodcastSkipBack15() } catch (_: Exception) {}
                POD_SKIP_FORWARD_30 -> try { nativeOnPodcastSkipForward30() } catch (_: Exception) {}
                POD_SPEED_CYCLE -> try { nativeOnPodcastSpeedCycle() } catch (_: Exception) {}
            }
            return Futures.immediateFuture(SessionResult(SessionResult.RESULT_SUCCESS))
        }
    }

    /** Fetch a cover image from the server, scale it down, and return JPEG bytes. */
    private fun fetchScaledCoverBytes(url: String, api: OrbApiClient): ByteArray? {
        return try {
            val conn = URL(url).openConnection() as HttpURLConnection
            conn.connectTimeout = 5000
            conn.readTimeout = 8000
            conn.setRequestProperty("Authorization", "Bearer ${api.token}")
            if (conn.responseCode != 200) { conn.disconnect(); return null }
            val bm = BitmapFactory.decodeStream(conn.inputStream)
            conn.disconnect()
            if (bm == null) return null
            val scaled = Bitmap.createScaledBitmap(bm, 128, 128, true)
            if (scaled !== bm) bm.recycle()
            val out = ByteArrayOutputStream()
            scaled.compress(Bitmap.CompressFormat.JPEG, 80, out)
            scaled.recycle()
            out.toByteArray()
        } catch (e: Exception) {
            Log.w(TAG, "fetchScaledCoverBytes failed: ${e.message}")
            null
        }
    }

    /**
     * Custom BitmapLoader that adds auth headers for server cover art requests,
     * checks offline cover files, and handles file:// URIs.
     * Wired into the MediaLibrarySession so the notification system loads artwork correctly.
     */
    private inner class AuthBitmapLoader : androidx.media3.common.util.BitmapLoader {
        override fun supportsMimeType(mimeType: String): Boolean = mimeType.startsWith("image/")

        override fun decodeBitmap(data: ByteArray): ListenableFuture<Bitmap> {
            val future = com.google.common.util.concurrent.SettableFuture.create<Bitmap>()
            pool.execute {
                try {
                    val bm = BitmapFactory.decodeByteArray(data, 0, data.size)
                    if (bm != null) future.set(bm) else future.setException(Exception("decode failed"))
                } catch (e: Exception) { future.setException(e) }
            }
            return future
        }

        override fun loadBitmap(uri: Uri): ListenableFuture<Bitmap> {
            val future = com.google.common.util.concurrent.SettableFuture.create<Bitmap>()
            pool.execute {
                try {
                    val uriStr = uri.toString()

                    // content:// URI (FileProvider — offline cover in browse list)
                    if (uri.scheme == "content") {
                        val stream = contentResolver.openInputStream(uri)
                        if (stream != null) {
                            val bm = BitmapFactory.decodeStream(stream)
                            stream.close()
                            if (bm != null) { future.set(bm); return@execute }
                        }
                        future.setException(Exception("content URI decode failed"))
                        return@execute
                    }

                    // file:// URI (offline cover)
                    if (uri.scheme == "file") {
                        val bm = BitmapFactory.decodeFile(uri.path)
                        if (bm != null) { future.set(bm); return@execute }
                        future.setException(Exception("decode file failed"))
                        return@execute
                    }

                    // Check local offline cover by album ID extracted from URL
                    // Match only album covers (not artist/podcast/audiobook covers)
                    val albumIdMatch = Regex("/covers/([^/?]+)(?:\\?|$)").find(uriStr)
                    albumIdMatch?.groupValues?.get(1)?.let { albumId ->
                        // Only use if it doesn't contain special album type prefixes
                        if (!albumId.startsWith("artist") && !albumId.startsWith("podcast") && !albumId.startsWith("audiobook")) {
                            val offlineCover = File(File(filesDir, OFFLINE_COVERS_DIR), albumId)
                            if (offlineCover.exists()) {
                                val bm = BitmapFactory.decodeFile(offlineCover.absolutePath)
                                if (bm != null) { future.set(bm); return@execute }
                            }
                        }
                    }

                    // HTTP fetch with auth header
                    val conn = URL(uriStr).openConnection() as HttpURLConnection
                    conn.connectTimeout = 8000
                    conn.readTimeout = 15000
                    apiClient?.token?.let { conn.setRequestProperty("Authorization", "Bearer $it") }
                    try {
                        val responseCode = conn.responseCode
                        if (responseCode == 200) {
                            val bm = BitmapFactory.decodeStream(conn.inputStream)
                            conn.disconnect()
                            if (bm != null) future.set(bm)
                            else future.setException(Exception("decode stream failed"))
                        } else {
                            conn.disconnect()
                            future.setException(Exception("HTTP $responseCode"))
                        }
                    } catch (e: Exception) {
                        conn.disconnect()
                        future.setException(Exception("HTTP request failed: ${e.message}"))
                    }
                } catch (e: Exception) { future.setException(e) }
            }
            return future
        }
    }

    private fun buildCustomLayout(): ImmutableList<CommandButton> {
        return when {
            isAudiobook -> ImmutableList.of(
                quickCommand("Skip back 15s", R.drawable.ic_replay_15, AB_SKIP_BACK_15),
                quickCommand("Skip forward 15s", R.drawable.ic_forward_15, AB_SKIP_FORWARD_15),
                quickCommand("Speed", speedIcons[currentSpeed] ?: R.drawable.ic_playback_speed_1, AB_SPEED_CYCLE),
                quickCommand("Chapter", R.drawable.ic_skip_to_chapter_start, AB_CHAPTER_START)
            )
            isPodcast -> ImmutableList.of(
                quickCommand("Back 15s", R.drawable.ic_replay_15, POD_SKIP_BACK_15),
                quickCommand("Forward 30s", R.drawable.ic_forward_30, POD_SKIP_FORWARD_30),
                quickCommand("Speed", speedIcons[currentSpeed] ?: R.drawable.ic_playback_speed_1, POD_SPEED_CYCLE)
            )
            else -> {
                val shuffleIcon = if (isShuffled) R.drawable.ic_shuffle else R.drawable.ic_shuffle_off
                val favoriteIcon = if (isFavorited) R.drawable.ic_heart_filled else R.drawable.ic_heart_outline
                ImmutableList.of(quickCommand("Shuffle", shuffleIcon, SHUFFLE_COMMAND), quickCommand("Favorite", favoriteIcon, FAVORITE_COMMAND))
            }
        }
    }

    private fun quickCommand(name: String, icon: Int, action: String) = CommandButton.Builder()
        .setDisplayName(name).setIconResId(icon).setSessionCommand(SessionCommand(action, Bundle.EMPTY)).build()

    private fun initEqualizer(audioSessionId: Int) {
        try {
            equalizer?.release()
            equalizer = Equalizer(0, audioSessionId).also { it.enabled = true }
        } catch (e: Exception) { equalizer = null }
    }

    private fun applyEQBands(bandsJson: String) {
        val eq = equalizer ?: return
        try {
            val arr = JSONArray(bandsJson)
            val numBands = eq.numberOfBands.toInt()
            val levelRange = eq.bandLevelRange
            for (i in 0 until numBands) {
                val centerHz = eq.getCenterFreq(i.toShort()) / 1000.0
                var closestGainDb = 0.0
                var minDist = Double.MAX_VALUE
                for (j in 0 until arr.length()) {
                    val band = arr.getJSONObject(j)
                    val freq = band.getDouble("frequency")
                    val dist = Math.abs(Math.log(freq) - Math.log(centerHz))
                    if (dist < minDist) { minDist = dist; closestGainDb = band.getDouble("gain") }
                }
                val clamped = (closestGainDb * 100).toInt().toShort().coerceIn(levelRange[0], levelRange[1])
                eq.setBandLevel(i.toShort(), clamped)
            }
        } catch (_: Exception) {}
    }

    /**
     * Called from Car App Library screens (Android Auto) to play a track
     * and sync the JS frontend.
     */
    fun handlePlayFromAuto(url: String, title: String?, artist: String?, coverUrl: String?, trackId: String) {
        handlePlay(url, title, artist, coverUrl)
        try { nativeOnExternalPlay(trackId) } catch (_: Exception) {}
    }

    fun handlePlay(url: String, title: String?, artist: String?, coverUrl: String?) {
        if (nativeAutoAdvancedUrl == url) { nativeAutoAdvancedUrl = null; return }
        nativeAutoAdvancedUrl = null
        doHandlePlay(url, title, artist, coverUrl)
    }

    private fun doHandlePlay(url: String, title: String?, artist: String?, coverUrl: String?) {
        mediaLibrarySession?.setCustomLayout(buildCustomLayout())
        crossfadeTriggered = false
        if (crossfadeEnabled && crossfadeSecs > 0 && player?.isPlaying == true) {
            handlePlayWithCrossfade(url, title, artist, coverUrl)
        } else {
            handlePlayDirect(url, title, artist, coverUrl)
        }
    }

    private fun handlePlayDirect(url: String, title: String?, artist: String?, coverUrl: String?) {
        crossfadeRunnable?.let { mainHandler.removeCallbacks(it) }
        crossfadePlayer?.release()
        crossfadePlayer = null
        val item = MediaItem.Builder().setMediaId(url).setUri(url)
            .setMediaMetadata(buildMetadata(title, artist).buildUpon().setArtworkUri(coverUrl?.let { Uri.parse(it) }).build()).build()
        requestAudioFocus()
        player?.apply { volume = 1f; setMediaItem(item); prepare(); play() }
        mediaLibrarySession?.player = wrappedPlayer ?: player!!
    }

    private fun handlePlayWithCrossfade(url: String, title: String?, artist: String?, coverUrl: String?) {
        crossfadeRunnable?.let { mainHandler.removeCallbacks(it) }
        crossfadePlayer?.release()
        val currentItem = player?.currentMediaItem
        val currentPos = player?.currentPosition ?: 0L
        if (currentItem != null) {
            val crossfadeDsf = dataSourceFactory
            crossfadePlayer = ExoPlayer.Builder(this, DefaultRenderersFactory(this).setEnableAudioFloatOutput(true))
                .apply { if (crossfadeDsf != null) setMediaSourceFactory(DefaultMediaSourceFactory(this@MediaService).setDataSourceFactory(crossfadeDsf)) }
                .build().apply {
                    setAudioAttributes(androidx.media3.common.AudioAttributes.Builder().setUsage(androidx.media3.common.C.USAGE_MEDIA).setContentType(androidx.media3.common.C.AUDIO_CONTENT_TYPE_MUSIC).build(), false)
                    volume = 1f; setMediaItem(currentItem); seekTo(currentPos); prepare(); play()
                }
        }
        requestAudioFocus()
        val newItem = MediaItem.Builder().setMediaId(url).setUri(url).setMediaMetadata(buildMetadata(title, artist).buildUpon().setArtworkUri(coverUrl?.let { Uri.parse(it) }).build()).build()
        player?.apply { volume = 0f; setMediaItem(newItem); prepare(); play() }
        mediaLibrarySession?.player = wrappedPlayer ?: player!!
        val totalMs = (crossfadeSecs * 1000f).toLong().coerceAtLeast(500L)
        val steps = 20; val stepMs = totalMs / steps; var step = 0
        crossfadeRunnable = object : Runnable {
            override fun run() {
                step++; val progress = step.toFloat() / steps
                player?.volume = progress; crossfadePlayer?.volume = 1f - progress
                if (step < steps) mainHandler.postDelayed(this, stepMs)
                else { player?.volume = 1f; crossfadePlayer?.release(); crossfadePlayer = null; crossfadeRunnable = null }
            }
        }.also { mainHandler.postDelayed(it, stepMs) }
    }

    private fun buildMetadata(title: String?, artist: String?): MediaMetadata = MediaMetadata.Builder()
        .setTitle(title ?: "Unknown").setArtist(artist ?: "Unknown").setIsPlayable(true).setIsBrowsable(false).build()

    /** Build a display-only MediaItem for a queue entry (no playback URI needed). */
    private fun buildQueueMediaItem(track: QueueTrack): MediaItem {
        val artUri = track.albumId?.let { buildCoverUrlForAlbum(it)?.let { u -> Uri.parse(u) } }
        return MediaItem.Builder()
            .setMediaId("track:${track.trackId}")
            .setMediaMetadata(
                MediaMetadata.Builder()
                    .setTitle(track.title)
                    .setArtist(track.artist)
                    .setIsPlayable(true)
                    .setArtworkUri(artUri)
                    .build()
            )
            .build()
    }

    // Artwork loading is now handled by AuthBitmapLoader on the MediaLibrarySession.

    private fun advanceNativeQueue() {
        var nextTrack: QueueTrack?
        var nextIndex: Int

        synchronized(nativeQueue) {
            if (nativeQueue.isEmpty()) {
                try { nativeOnNext() } catch (_: Exception) {}
                return
            }

            when (nativeRepeatMode) {
                "one" -> {
                    nextIndex = nativeQueueIndex
                    nextTrack = nativeQueue.getOrNull(nextIndex)
                }
                "all" -> {
                    nextIndex = if (nativeQueueIndex < nativeQueue.size - 1) {
                        nativeQueueIndex + 1
                    } else {
                        0
                    }
                    nextTrack = nativeQueue.getOrNull(nextIndex)
                }
                else -> { // "off"
                    nextIndex = nativeQueueIndex + 1
                    nextTrack = if (nextIndex < nativeQueue.size) {
                        nativeQueue[nextIndex]
                    } else {
                        null
                    }
                }
            }
        }

        if (nextTrack == null && nativeAutoplayEnabled && nativeRepeatMode == "off") {
            val lastTrack = synchronized(nativeQueue) { nativeQueue.lastOrNull() }
            if (lastTrack != null) {
                fetchAndPlayAutoplay(lastTrack.trackId)
            }
        }

        val trackToPlay = nextTrack
        if (trackToPlay != null) {
            nativeQueueIndex = nextIndex
            val url = buildStreamUrlForTrack(trackToPlay.trackId)
            if (url != null) {
                val coverUrl = buildCoverUrlForAlbum(trackToPlay.albumId)
                doHandlePlay(url, trackToPlay.title, trackToPlay.artist, coverUrl)
            }
            try { nativeOnQueueAdvanced(nativeQueueIndex) } catch (_: Exception) {}
        } else {
            try { nativeOnNext() } catch (_: Exception) {}
        }
    }

    private fun fetchAndPlayAutoplay(afterTrackId: String) {
        val client = apiClient ?: return
        val existingIds = synchronized(nativeQueue) { nativeQueue.map { it.trackId } }

        pool.execute {
            try {
                val tracks = client.autoplay(afterTrackId, existingIds, 10)
                if (tracks.isEmpty()) return@execute

                val newQueueTracks = tracks.map { t ->
                    QueueTrack(
                        trackId = t.id,
                        title = t.title,
                        artist = t.artistName,
                        albumId = t.albumId,
                    )
                }

                mainHandler.post {
                    synchronized(nativeQueue) {
                        nativeQueue.addAll(newQueueTracks)
                    }
                    val nextIdx = nativeQueueIndex + 1
                    val track = synchronized(nativeQueue) { nativeQueue.getOrNull(nextIdx) }
                    if (track != null) {
                        nativeQueueIndex = nextIdx
                        val url = buildStreamUrlForTrack(track.trackId)
                        if (url != null) {
                            val coverUrl = buildCoverUrlForAlbum(track.albumId)
                            doHandlePlay(url, track.title, track.artist, coverUrl)
                        }
                        try { nativeOnQueueAdvanced(nativeQueueIndex) } catch (_: Exception) {}
                    }
                }
            } catch (e: Exception) {
                Log.w(TAG, "Native autoplay fetch failed: ${e.message}")
            }
        }
    }

    private fun buildStreamUrlForTrack(trackId: String): String? {
        val offlinePath = getOfflineFilePath(trackId)
        if (offlinePath != null) return "file://$offlinePath"
        val client = apiClient ?: return null
        return client.streamUrl(trackId)
    }

    private fun buildCoverUrlForAlbum(albumId: String?): String? {
        if (albumId == null) return null
        // Use offline cover if available
        val offlineCover = File(File(filesDir, OFFLINE_COVERS_DIR), albumId)
        if (offlineCover.exists()) return Uri.fromFile(offlineCover).toString()
        // Cover endpoints are public — no token needed in the URL.
        // AuthBitmapLoader adds the auth header if needed.
        val client = apiClient ?: return null
        return client.coverUrl(albumId)
    }

    /**
     * Fetch album tracks and set them as the native queue so playback continues
     * through the album when a single track is played from Android Auto.
     */
    private fun loadAlbumQueueForTrack(trackId: String, albumId: String) {
        val api = apiClient ?: return
        try {
            val detail = api.albumDetail(albumId) ?: return
            val tracks = detail.tracks
            if (tracks.isEmpty()) return

            val newQueue = tracks.map { t ->
                QueueTrack(
                    trackId = t.id,
                    title = t.title,
                    artist = t.artistName,
                    albumId = t.albumId
                )
            }
            val index = newQueue.indexOfFirst { it.trackId == trackId }.coerceAtLeast(0)

            mainHandler.post {
                synchronized(nativeQueue) {
                    nativeQueue.clear()
                    nativeQueue.addAll(newQueue)
                }
                nativeQueueIndex = index
                nativeAutoAdvancedUrl = null
            }
        } catch (e: Exception) {
            Log.w(TAG, "loadAlbumQueueForTrack failed: ${e.message}")
        }
    }

    fun runOnUiThread(action: Runnable) { mainHandler.post(action) }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaLibrarySession? = mediaLibrarySession

    private fun optNullableString(o: JSONObject, key: String): String? = if (o.isNull(key)) null else o.optString(key, null)

    // RESTORED: Audio focus management functions
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

    // ── Offline favorites ──────────────────────────────────────────────────

    /** Load favorite IDs and pending ops from SharedPreferences into memory. */
    private fun loadFavoritesFromPrefs() {
        val prefs = getSharedPreferences(FAVS_PREFS, Context.MODE_PRIVATE)
        val idsJson = prefs.getString(FAVS_KEY_IDS, null)
        if (idsJson != null) {
            try {
                val arr = JSONArray(idsJson)
                for (i in 0 until arr.length()) favoriteIds.add(arr.getString(i))
            } catch (_: Exception) {}
        }
        val pendingJson = prefs.getString(FAVS_KEY_PENDING, null)
        if (pendingJson != null) {
            try {
                val arr = JSONArray(pendingJson)
                for (i in 0 until arr.length()) pendingFavOps.add(arr.getString(i))
            } catch (_: Exception) {}
        }
    }

    /** Persist current favorite IDs to SharedPreferences. */
    private fun saveFavoriteIds() {
        val arr = JSONArray()
        favoriteIds.forEach { arr.put(it) }
        getSharedPreferences(FAVS_PREFS, Context.MODE_PRIVATE).edit()
            .putString(FAVS_KEY_IDS, arr.toString())
            .apply()
    }

    /** Persist pending ops to SharedPreferences. */
    private fun savePendingOps() {
        val arr = JSONArray()
        synchronized(pendingFavOps) { pendingFavOps.forEach { arr.put(it) } }
        getSharedPreferences(FAVS_PREFS, Context.MODE_PRIVATE).edit()
            .putString(FAVS_KEY_PENDING, arr.toString())
            .apply()
    }

    /**
     * Toggle favorite for the currently playing track.
     * Updates local state immediately, then tries to sync with the backend.
     * If the backend is unreachable, the operation is queued for later.
     */
    fun toggleFavoriteForCurrentTrack() {
        val trackId = synchronized(nativeQueue) {
            nativeQueue.getOrNull(nativeQueueIndex)?.trackId
        } ?: return

        val nowFavorited = if (favoriteIds.contains(trackId)) {
            favoriteIds.remove(trackId)
            pendingFavOps.removeAll { it == "add:$trackId" }
            pendingFavOps.add("remove:$trackId")
            false
        } else {
            favoriteIds.add(trackId)
            pendingFavOps.removeAll { it == "remove:$trackId" }
            pendingFavOps.add("add:$trackId")
            true
        }

        isFavorited = nowFavorited
        saveFavoriteIds()
        savePendingOps()
        mediaLibrarySession?.setCustomLayout(buildCustomLayout())

        // Also notify the JS frontend so it can update its UI if open
        try { nativeOnFavoriteToggle() } catch (_: Exception) {}

        // Try to sync immediately on a background thread
        pool.execute { syncPendingFavorites() }
    }

    /**
     * Flush all pending favorite operations to the backend.
     * Called on a background thread. Operations that succeed are removed;
     * failures stay queued for the next attempt.
     */
    private fun syncPendingFavorites() {
        val api = apiClient ?: return
        val snapshot = synchronized(pendingFavOps) { pendingFavOps.toList() }
        if (snapshot.isEmpty()) return

        val synced = mutableListOf<String>()
        for (op in snapshot) {
            val parts = op.split(":", limit = 2)
            if (parts.size != 2) { synced.add(op); continue }
            val (action, trackId) = parts
            val ok = when (action) {
                "add" -> api.addFavorite(trackId)
                "remove" -> api.removeFavorite(trackId)
                else -> { synced.add(op); continue }
            }
            if (ok) {
                synced.add(op)
                Log.d(TAG, "Synced favorite op: $op")
            } else {
                // Stop on first failure — backend probably unreachable
                Log.d(TAG, "Favorite sync failed at: $op, will retry later")
                break
            }
        }
        if (synced.isNotEmpty()) {
            synchronized(pendingFavOps) { pendingFavOps.removeAll(synced.toSet()) }
            savePendingOps()
        }
    }

    /**
     * Pull the full favorite ID set from the backend and merge with local state.
     * Also flushes any pending operations first.
     */
    fun refreshFavoritesFromBackend() {
        val api = apiClient ?: return
        pool.execute {
            // First push any pending changes
            syncPendingFavorites()
            // Then pull the authoritative set
            val remoteIds = api.fetchFavoriteIds()
            if (remoteIds.isNotEmpty() || pendingFavOps.isEmpty()) {
                favoriteIds.clear()
                favoriteIds.addAll(remoteIds)
                // Re-add any still-pending adds (not yet synced)
                synchronized(pendingFavOps) {
                    for (op in pendingFavOps) {
                        val parts = op.split(":", limit = 2)
                        if (parts.size == 2 && parts[0] == "add") favoriteIds.add(parts[1])
                        if (parts.size == 2 && parts[0] == "remove") favoriteIds.remove(parts[1])
                    }
                }
                saveFavoriteIds()
                // Update the button state for the current track
                val currentTrackId = synchronized(nativeQueue) {
                    nativeQueue.getOrNull(nativeQueueIndex)?.trackId
                }
                if (currentTrackId != null) {
                    val fav = favoriteIds.contains(currentTrackId)
                    mainHandler.post {
                        isFavorited = fav
                        mediaLibrarySession?.setCustomLayout(buildCustomLayout())
                    }
                }
            }
            Log.d(TAG, "Favorites refreshed from backend: ${favoriteIds.size} tracks")
        }
    }

    /**
     * Update the favorite button state for the given track based on local favorite set.
     */
    fun updateFavoriteStateForTrack(trackId: String) {
        val fav = favoriteIds.contains(trackId)
        isFavorited = fav
        mediaLibrarySession?.setCustomLayout(buildCustomLayout())
    }

    override fun onDestroy() {
        abandonAudioFocus()
        mainHandler.removeCallbacksAndMessages(null)
        crossfadePlayer?.release()
        equalizer?.release()
        mediaLibrarySession?.run { player.release(); release() }
        instance = null
        super.onDestroy()
    }
}