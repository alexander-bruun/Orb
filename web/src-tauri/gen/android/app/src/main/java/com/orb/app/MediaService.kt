package com.orb.app

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.AudioAttributes
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
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.session.CommandButton
import androidx.media3.session.DefaultMediaNotificationProvider
import androidx.media3.session.LibraryResult
import androidx.media3.session.MediaLibraryService
import androidx.media3.session.MediaSession
import androidx.media3.session.SessionCommand
import androidx.media3.session.SessionResult
import androidx.media3.session.MediaConstants
import com.google.common.collect.ImmutableList
import com.google.common.util.concurrent.Futures
import com.google.common.util.concurrent.ListenableFuture
import com.google.common.util.concurrent.SettableFuture
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.net.URL
import java.util.concurrent.Executors

@OptIn(UnstableApi::class)
class MediaService : MediaLibraryService() {

    private var player: ExoPlayer? = null
    private var mediaSession: MediaLibrarySession? = null

    // Custom command identifiers
    private val SHUFFLE_COMMAND = "SHUFFLE_TOGGLE"
    private val FAVORITE_COMMAND = "FAVORITE_TOGGLE"

    // State (synced with frontend)
    private var isShuffled = false
    private var isFavorited = false

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

    private var apiClient: OrbApiClient? = null
    private val ioExecutor = Executors.newCachedThreadPool()

    companion object {
        // Browse node IDs
        private const val TAG = "MediaService"
        private const val ROOT_ID = "root"
        private const val RECENTLY_PLAYED_ID = "recently_played"
        private const val RECENTLY_ADDED_ID = "recently_added"
        private const val PLAYLISTS_ID = "playlists"
        private const val FAVORITES_ID = "favorites"
        private const val ALBUMS_ID = "albums"
        private const val ARTISTS_ID = "artists"
        private const val MOST_PLAYED_ID = "most_played"
        private const val DOWNLOADS_ID = "downloads"
        private const val ALBUM_PREFIX = "album:"
        private const val ARTIST_PREFIX = "artist:"
        private const val PLAYLIST_PREFIX = "playlist:"
        private const val TRACK_PREFIX = "track:"
        private const val OFFLINE_TRACK_PREFIX = "offline:"
        private const val PREFS_NAME = "orb_downloads"
        private const val PREFS_KEY_METADATA = "download_metadata"
        private const val OFFLINE_DIR = "offline_audio"
        private const val OFFLINE_COVERS_DIR = "offline_covers"

        @Volatile
        private var instance: MediaService? = null

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
        fun setApiCredentials(baseUrl: String, token: String) {
            val svc = instance ?: return
            if (svc.apiClient == null) {
                svc.apiClient = OrbApiClient(baseUrl, token)
            } else {
                svc.apiClient?.updateCredentials(baseUrl, token)
            }
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
         * Save an audio file to internal storage for offline playback.
         * Called from Rust after downloading a track.
         * Returns the absolute file path.
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
                        // Normal end — no crossfade was triggered early.
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
            override fun getAvailableCommands(): Player.Commands =
                super.getAvailableCommands().buildUpon()
                    .add(Player.COMMAND_SEEK_TO_NEXT)
                    .add(Player.COMMAND_SEEK_TO_PREVIOUS)
                    .build()

            override fun isCommandAvailable(command: Int): Boolean =
                if (command == Player.COMMAND_SEEK_TO_NEXT ||
                    command == Player.COMMAND_SEEK_TO_PREVIOUS) true
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

        val sessionActivityIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )

        mediaSession = MediaLibrarySession.Builder(this, forwardingPlayer, LibraryCallback())
            .setSessionActivity(sessionActivityIntent)
            .build()

        mediaSession?.setCustomLayout(buildCustomLayout())

        // Initialize lastNotifiedVolume so the player listener can detect changes
        lastNotifiedVolume = getVolume()

        // Start the volume polling loop
        mainHandler.post(volumeChecker)
    }

    // ── MediaLibrarySession.Callback ─────────────────────────────────────────

    private inner class LibraryCallback : MediaLibrarySession.Callback {

        override fun onConnect(
            session: MediaSession,
            controller: MediaSession.ControllerInfo
        ): MediaSession.ConnectionResult {
            val sessionCommands = MediaSession.ConnectionResult.DEFAULT_SESSION_AND_LIBRARY_COMMANDS.buildUpon()
                .add(SessionCommand(SHUFFLE_COMMAND, Bundle.EMPTY))
                .add(SessionCommand(FAVORITE_COMMAND, Bundle.EMPTY))
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
            }
            return Futures.immediateFuture(SessionResult(SessionResult.RESULT_SUCCESS))
        }

        override fun onGetLibraryRoot(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            params: LibraryParams?
        ): ListenableFuture<LibraryResult<MediaItem>> {
            // Advertise content style support so Android Auto can render
            // grid and list sections based on per-node hints.
            val extras = Bundle().apply {
                putInt(
                    MediaConstants.EXTRAS_KEY_CONTENT_STYLE_BROWSABLE,
                    MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM
                )
                putInt(
                    MediaConstants.EXTRAS_KEY_CONTENT_STYLE_PLAYABLE,
                    MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM
                )
            }
            val root = MediaItem.Builder()
                .setMediaId(ROOT_ID)
                .setMediaMetadata(
                    MediaMetadata.Builder()
                        .setTitle("Orb")
                        .setIsBrowsable(true)
                        .setIsPlayable(false)
                        .setMediaType(MediaMetadata.MEDIA_TYPE_FOLDER_MIXED)
                        .setExtras(extras)
                        .build()
                )
                .build()
            return Futures.immediateFuture(LibraryResult.ofItem(root, params))
        }

        override fun onGetChildren(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            parentId: String,
            page: Int,
            pageSize: Int,
            params: LibraryParams?
        ): ListenableFuture<LibraryResult<ImmutableList<MediaItem>>> {
            val future = SettableFuture.create<LibraryResult<ImmutableList<MediaItem>>>()
            ioExecutor.execute {
                try {
                    val items = when {
                        parentId == ROOT_ID -> buildRootChildren()
                        parentId == RECENTLY_PLAYED_ID -> buildRecentlyPlayedChildren()
                        parentId == RECENTLY_ADDED_ID -> buildRecentlyAddedChildren()
                        parentId == MOST_PLAYED_ID -> buildMostPlayedChildren()
                        parentId == PLAYLISTS_ID -> buildPlaylistsChildren()
                        parentId == FAVORITES_ID -> buildFavoritesChildren()
                        parentId == DOWNLOADS_ID -> buildDownloadsChildren()
                        parentId == ALBUMS_ID -> buildAlbumsChildren(page, pageSize)
                        parentId == ARTISTS_ID -> buildArtistsChildren(page, pageSize)
                        parentId.startsWith(ALBUM_PREFIX) -> buildAlbumTracksChildren(parentId.removePrefix(ALBUM_PREFIX))
                        parentId.startsWith(ARTIST_PREFIX) -> buildArtistAlbumsChildren(parentId.removePrefix(ARTIST_PREFIX))
                        parentId.startsWith(PLAYLIST_PREFIX) -> buildPlaylistTracksChildren(parentId.removePrefix(PLAYLIST_PREFIX))
                        else -> ImmutableList.of()
                    }
                    future.set(LibraryResult.ofItemList(items, params))
                } catch (e: Exception) {
                    future.set(LibraryResult.ofItemList(ImmutableList.of(), params))
                }
            }
            return future
        }

        override fun onGetItem(
            session: MediaLibrarySession,
            browser: MediaSession.ControllerInfo,
            mediaId: String
        ): ListenableFuture<LibraryResult<MediaItem>> {
            // Return a generic item — Android Auto primarily uses onGetChildren
            val item = MediaItem.Builder()
                .setMediaId(mediaId)
                .setMediaMetadata(
                    MediaMetadata.Builder()
                        .setTitle(mediaId)
                        .setIsBrowsable(false)
                        .setIsPlayable(true)
                        .build()
                )
                .build()
            return Futures.immediateFuture(LibraryResult.ofItem(item, null))
        }

        override fun onAddMediaItems(
            mediaSession: MediaSession,
            controller: MediaSession.ControllerInfo,
            mediaItems: MutableList<MediaItem>
        ): ListenableFuture<MutableList<MediaItem>> {
            // Called when Android Auto wants to play an item from the browse tree.
            // Resolve track IDs to playable stream URLs or local file URIs.
            val resolved = mediaItems.map { item ->
                val mediaId = item.mediaId
                when {
                    mediaId.startsWith(OFFLINE_TRACK_PREFIX) -> {
                        val trackId = mediaId.removePrefix(OFFLINE_TRACK_PREFIX)
                        val offlineFile = File(File(filesDir, OFFLINE_DIR), trackId)
                        if (offlineFile.exists()) {
                            item.buildUpon()
                                .setUri(Uri.fromFile(offlineFile))
                                .build()
                        } else {
                            // Fallback to stream if file missing but API available
                            val client = apiClient
                            if (client != null) {
                                item.buildUpon()
                                    .setUri(client.streamUrl(trackId))
                                    .build()
                            } else item
                        }
                    }
                    mediaId.startsWith(TRACK_PREFIX) -> {
                        val trackId = mediaId.removePrefix(TRACK_PREFIX)
                        // Prefer offline file if available
                        val offlineFile = File(File(filesDir, OFFLINE_DIR), trackId)
                        if (offlineFile.exists()) {
                            item.buildUpon()
                                .setUri(Uri.fromFile(offlineFile))
                                .build()
                        } else {
                            val client = apiClient
                            if (client != null) {
                                item.buildUpon()
                                    .setUri(client.streamUrl(trackId))
                                    .build()
                            } else item
                        }
                    }
                    else -> item
                }
            }.toMutableList()

            return Futures.immediateFuture(resolved)
        }
    }

    // ── Browse tree builders ─────────────────────────────────────────────────

    private fun isApiReachable(): Boolean {
        val client = apiClient ?: return false
        return client.isReachable()
    }

    private fun buildRootChildren(): ImmutableList<MediaItem> {
        val hasDownloads = getDownloadMetadata().isNotEmpty()
        val online = apiClient != null && try { isApiReachable() } catch (_: Exception) { false }

        if (!online) {
            // Offline mode: only show downloads
            return if (hasDownloads) {
                ImmutableList.of(
                    browsableItem(DOWNLOADS_ID, "Downloads", MediaMetadata.MEDIA_TYPE_FOLDER_MIXED)
                )
            } else {
                ImmutableList.of()
            }
        }

        // Spotify-style recommendation feed: content-driven sections with
        // per-node content style hints for grid vs list rendering.
        val items = mutableListOf(
            // Grid sections — album art cards
            styledBrowsableItem(
                RECENTLY_PLAYED_ID, "Jump back in",
                MediaMetadata.MEDIA_TYPE_FOLDER_ALBUMS,
                childBrowsableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM,
                childPlayableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM
            ),
            styledBrowsableItem(
                RECENTLY_ADDED_ID, "Recently added",
                MediaMetadata.MEDIA_TYPE_FOLDER_ALBUMS,
                childBrowsableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM,
                childPlayableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM
            ),
            // List section — track titles scannable
            styledBrowsableItem(
                MOST_PLAYED_ID, "Most played",
                MediaMetadata.MEDIA_TYPE_FOLDER_MIXED,
                childPlayableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM
            ),
            styledBrowsableItem(
                FAVORITES_ID, "Favorites",
                MediaMetadata.MEDIA_TYPE_FOLDER_MIXED,
                childPlayableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM
            ),
            // Grid section — playlist artwork cards
            styledBrowsableItem(
                PLAYLISTS_ID, "Your playlists",
                MediaMetadata.MEDIA_TYPE_FOLDER_PLAYLISTS,
                childBrowsableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM
            ),
            // Category grid — large visual anchors for browsing
            styledBrowsableItem(
                ARTISTS_ID, "Artists",
                MediaMetadata.MEDIA_TYPE_FOLDER_ARTISTS,
                childBrowsableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_CATEGORY_GRID_ITEM
            ),
            styledBrowsableItem(
                ALBUMS_ID, "Albums",
                MediaMetadata.MEDIA_TYPE_FOLDER_ALBUMS,
                childBrowsableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_GRID_ITEM
            ),
        )
        if (hasDownloads) {
            items.add(styledBrowsableItem(
                DOWNLOADS_ID, "Downloads",
                MediaMetadata.MEDIA_TYPE_FOLDER_MIXED,
                childPlayableStyle = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM
            ))
        }
        return ImmutableList.copyOf(items)
    }

    private fun buildRecentlyPlayedChildren(): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val albums = client.recentlyPlayedAlbums()
        return ImmutableList.copyOf(albums.map { albumToMediaItem(it, client) })
    }

    private fun buildRecentlyAddedChildren(): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val albums = client.recentlyAddedAlbums()
        return ImmutableList.copyOf(albums.map { albumToMediaItem(it, client) })
    }

    private fun buildPlaylistsChildren(): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val playlists = client.playlists()
        return ImmutableList.copyOf(playlists.map { playlist ->
            MediaItem.Builder()
                .setMediaId("$PLAYLIST_PREFIX${playlist.id}")
                .setMediaMetadata(
                    MediaMetadata.Builder()
                        .setTitle(playlist.name)
                        .setSubtitle(playlist.description)
                        .setArtworkUri(Uri.parse(client.playlistCoverUrl(playlist.id)))
                        .setIsBrowsable(true)
                        .setIsPlayable(false)
                        .setMediaType(MediaMetadata.MEDIA_TYPE_PLAYLIST)
                        .build()
                )
                .build()
        })
    }

    private fun buildFavoritesChildren(): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val tracks = client.favorites()
        return ImmutableList.copyOf(tracks.map { trackToMediaItem(it, client) })
    }

    private fun buildAlbumsChildren(page: Int, pageSize: Int): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val offset = page * pageSize
        val albums = client.albums(limit = pageSize, offset = offset)
        return ImmutableList.copyOf(albums.map { albumToMediaItem(it, client) })
    }

    private fun buildAlbumTracksChildren(albumId: String): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val detail = client.albumDetail(albumId) ?: return ImmutableList.of()
        return ImmutableList.copyOf(detail.tracks.map { trackToMediaItem(it, client) })
    }

    private fun buildPlaylistTracksChildren(playlistId: String): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val detail = client.playlistDetail(playlistId) ?: return ImmutableList.of()
        return ImmutableList.copyOf(detail.tracks.map { trackToMediaItem(it, client) })
    }

    private fun buildDownloadsChildren(): ImmutableList<MediaItem> {
        val metadata = getDownloadMetadata()
        return ImmutableList.copyOf(metadata.map { dl ->
            val metadataBuilder = MediaMetadata.Builder()
                .setTitle(dl.title)
                .setArtist(dl.artistName)
                .setAlbumTitle(dl.albumName)
                .setIsBrowsable(false)
                .setIsPlayable(true)
                .setMediaType(MediaMetadata.MEDIA_TYPE_MUSIC)

            if (dl.albumId != null) {
                // Prefer local cover art, fall back to server URL
                val localCover = File(File(filesDir, OFFLINE_COVERS_DIR), dl.albumId)
                if (localCover.exists()) {
                    metadataBuilder.setArtworkUri(Uri.fromFile(localCover))
                } else {
                    apiClient?.let { client ->
                        metadataBuilder.setArtworkUri(Uri.parse(client.coverUrl(dl.albumId)))
                    }
                }
            }

            MediaItem.Builder()
                .setMediaId("$OFFLINE_TRACK_PREFIX${dl.trackId}")
                .setMediaMetadata(metadataBuilder.build())
                .build()
        })
    }

    private fun buildMostPlayedChildren(): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val tracks = client.mostPlayedTracks()
        return ImmutableList.copyOf(tracks.map { trackToMediaItem(it, client) })
    }

    private fun buildArtistsChildren(page: Int, pageSize: Int): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val offset = page * pageSize
        val artists = client.artists(limit = pageSize, offset = offset)
        return ImmutableList.copyOf(artists.map { artistToMediaItem(it, client) })
    }

    private fun buildArtistAlbumsChildren(artistId: String): ImmutableList<MediaItem> {
        val client = apiClient ?: return ImmutableList.of()
        val albums = client.artistAlbums(artistId)
        return ImmutableList.copyOf(albums.map { albumToMediaItem(it, client) })
    }

    private fun artistToMediaItem(artist: OrbApiClient.BrowseArtist, client: OrbApiClient): MediaItem {
        val metadataBuilder = MediaMetadata.Builder()
            .setTitle(artist.name)
            .setIsBrowsable(true)
            .setIsPlayable(false)
            .setMediaType(MediaMetadata.MEDIA_TYPE_ARTIST)

        if (artist.id.isNotEmpty()) {
            metadataBuilder.setArtworkUri(Uri.parse(client.artistCoverUrl(artist.id)))
        }

        return MediaItem.Builder()
            .setMediaId("$ARTIST_PREFIX${artist.id}")
            .setMediaMetadata(metadataBuilder.build())
            .build()
    }

    // ── Offline metadata ─────────────────────────────────────────────────────

    data class DownloadMeta(
        val trackId: String,
        val title: String,
        val artistName: String?,
        val albumName: String?,
        val albumId: String?
    )

    private fun getDownloadMetadata(): List<DownloadMeta> {
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
                    artistName = o.optString("artistName", null),
                    albumName = o.optString("albumName", null),
                    albumId = o.optString("albumId", null)
                )
            }
        } catch (e: Exception) {
            Log.w(TAG, "Failed to read download metadata: ${e.message}")
            emptyList()
        }
    }

    // ── MediaItem builders ───────────────────────────────────────────────────

    private fun browsableItem(id: String, title: String, mediaType: Int): MediaItem {
        return MediaItem.Builder()
            .setMediaId(id)
            .setMediaMetadata(
                MediaMetadata.Builder()
                    .setTitle(title)
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .setMediaType(mediaType)
                    .build()
            )
            .build()
    }

    /**
     * Build a browsable item with per-node content style hints.
     * These tell Android Auto how to render the *children* of this node
     * (grid cards vs list rows vs category tiles).
     */
    private fun styledBrowsableItem(
        id: String,
        title: String,
        mediaType: Int,
        childBrowsableStyle: Int = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM,
        childPlayableStyle: Int = MediaConstants.EXTRAS_VALUE_CONTENT_STYLE_LIST_ITEM
    ): MediaItem {
        val extras = Bundle().apply {
            putInt(MediaConstants.EXTRAS_KEY_CONTENT_STYLE_BROWSABLE, childBrowsableStyle)
            putInt(MediaConstants.EXTRAS_KEY_CONTENT_STYLE_PLAYABLE, childPlayableStyle)
        }
        return MediaItem.Builder()
            .setMediaId(id)
            .setMediaMetadata(
                MediaMetadata.Builder()
                    .setTitle(title)
                    .setIsBrowsable(true)
                    .setIsPlayable(false)
                    .setMediaType(mediaType)
                    .setExtras(extras)
                    .build()
            )
            .build()
    }

    private fun albumToMediaItem(album: OrbApiClient.BrowseAlbum, client: OrbApiClient): MediaItem {
        val metadataBuilder = MediaMetadata.Builder()
            .setTitle(album.title)
            .setArtist(album.artistName)
            .setIsBrowsable(true)
            .setIsPlayable(false)
            .setMediaType(MediaMetadata.MEDIA_TYPE_ALBUM)

        if (album.id.isNotEmpty()) {
            metadataBuilder.setArtworkUri(Uri.parse(client.coverUrl(album.id)))
        }

        return MediaItem.Builder()
            .setMediaId("$ALBUM_PREFIX${album.id}")
            .setMediaMetadata(metadataBuilder.build())
            .build()
    }

    private fun trackToMediaItem(track: OrbApiClient.BrowseTrack, client: OrbApiClient): MediaItem {
        val metadataBuilder = MediaMetadata.Builder()
            .setTitle(track.title)
            .setArtist(track.artistName)
            .setAlbumTitle(track.albumName)
            .setIsBrowsable(false)
            .setIsPlayable(true)
            .setMediaType(MediaMetadata.MEDIA_TYPE_MUSIC)

        if (track.albumId != null) {
            metadataBuilder.setArtworkUri(Uri.parse(client.coverUrl(track.albumId)))
        }

        return MediaItem.Builder()
            .setMediaId("$TRACK_PREFIX${track.id}")
            .setMediaMetadata(metadataBuilder.build())
            .build()
    }

    // ── Custom layout (shuffle + favorite buttons) ───────────────────────────

    private fun buildCustomLayout(): ImmutableList<CommandButton> {
        val shuffleIcon = if (isShuffled) R.drawable.ic_shuffle else R.drawable.ic_shuffle_off
        val shuffleButton = CommandButton.Builder(CommandButton.ICON_UNDEFINED)
            .setDisplayName("Shuffle")
            .setIconResId(shuffleIcon)
            .setSessionCommand(SessionCommand(SHUFFLE_COMMAND, Bundle.EMPTY))
            .build()

        val favoriteIcon = if (isFavorited) R.drawable.ic_heart_filled else R.drawable.ic_heart_outline
        val favoriteButton = CommandButton.Builder(CommandButton.ICON_UNDEFINED)
            .setDisplayName("Favorite")
            .setIconResId(favoriteIcon)
            .setSessionCommand(SessionCommand(FAVORITE_COMMAND, Bundle.EMPTY))
            .build()

        return ImmutableList.of(shuffleButton, favoriteButton)
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

    private fun handlePlay(url: String, title: String?, artist: String?, coverUrl: String?) {
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

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaLibrarySession? {
        return mediaSession
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
        player = null
        instance = null
        super.onDestroy()
    }
}
