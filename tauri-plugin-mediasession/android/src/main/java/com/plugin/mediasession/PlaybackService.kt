package com.plugin.mediasession

import android.content.Intent
import android.os.Handler
import android.os.Looper
import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.ForwardingPlayer
import androidx.media3.common.MediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.session.MediaSession
import androidx.media3.session.MediaSessionService

class PlaybackService : MediaSessionService() {

    private var exoPlayer: ExoPlayer? = null
    private var mediaSession: MediaSession? = null
    private val handler = Handler(Looper.getMainLooper())

    companion object {
        @Volatile
        var instance: PlaybackService? = null
            private set

        var eventCallback: EventCallback? = null

        private val readyCallbacks = mutableListOf<(PlaybackService) -> Unit>()

        fun whenReady(callback: (PlaybackService) -> Unit) {
            val existing = instance
            if (existing != null) {
                FileLog.d("whenReady: instance already exists, calling back immediately")
                callback(existing)
            } else {
                FileLog.d("whenReady: instance null, queuing callback (pending=${readyCallbacks.size + 1})")
                synchronized(readyCallbacks) {
                    readyCallbacks.add(callback)
                }
            }
        }

        private const val POSITION_INTERVAL_MS = 500L
    }

    interface EventCallback {
        fun onPlaybackStateChanged(state: String, positionMs: Long)
        fun onPositionUpdate(positionMs: Long, durationMs: Long)
        fun onMediaAction(action: String, seekPos: Long?)
        fun onTrackEnded()
    }

    private val positionTick = object : Runnable {
        override fun run() {
            exoPlayer?.let { p ->
                if (p.isPlaying) {
                    eventCallback?.onPositionUpdate(
                        p.currentPosition,
                        p.duration.coerceAtLeast(0)
                    )
                }
            }
            handler.postDelayed(this, POSITION_INTERVAL_MS)
        }
    }

    @OptIn(UnstableApi::class)
    override fun onCreate() {
        super.onCreate()
        FileLog.init(this)
        FileLog.d("PlaybackService.onCreate()")

        try {
            val player = ExoPlayer.Builder(this)
                .setHandleAudioBecomingNoisy(true)
                .setWakeMode(C.WAKE_MODE_NETWORK)
                .build()

            FileLog.d("ExoPlayer built successfully")

            player.setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(C.USAGE_MEDIA)
                    .setContentType(C.AUDIO_CONTENT_TYPE_MUSIC)
                    .build(),
                true
            )

            player.addListener(object : Player.Listener {
                override fun onPlaybackStateChanged(playbackState: Int) {
                    val state = when (playbackState) {
                        Player.STATE_IDLE -> "idle"
                        Player.STATE_BUFFERING -> "loading"
                        Player.STATE_READY -> if (player.playWhenReady) "playing" else "paused"
                        Player.STATE_ENDED -> "ended"
                        else -> "idle"
                    }
                    FileLog.d("Player.onPlaybackStateChanged: $state (raw=$playbackState, playWhenReady=${player.playWhenReady})")
                    eventCallback?.onPlaybackStateChanged(state, player.currentPosition)
                    if (playbackState == Player.STATE_ENDED) {
                        FileLog.d("Track ended, firing onTrackEnded")
                        eventCallback?.onTrackEnded()
                    }
                }

                override fun onIsPlayingChanged(isPlaying: Boolean) {
                    FileLog.d("Player.onIsPlayingChanged: isPlaying=$isPlaying, pos=${player.currentPosition}")
                    eventCallback?.onPlaybackStateChanged(
                        if (isPlaying) "playing" else "paused",
                        player.currentPosition
                    )
                }

                override fun onPlayerError(error: PlaybackException) {
                    FileLog.e("Player.onPlayerError: code=${error.errorCode}, message=${error.message}", error)
                    eventCallback?.onPlaybackStateChanged("idle", 0)
                }

                override fun onPositionDiscontinuity(
                    oldPosition: Player.PositionInfo,
                    newPosition: Player.PositionInfo,
                    reason: Int
                ) {
                    FileLog.d("Player.onPositionDiscontinuity: ${oldPosition.positionMs} -> ${newPosition.positionMs}, reason=$reason")
                    eventCallback?.onPositionUpdate(
                        newPosition.positionMs,
                        player.duration.coerceAtLeast(0)
                    )
                }
            })

            exoPlayer = player
            FileLog.d("ExoPlayer listener attached")

            val wrapper = object : ForwardingPlayer(player) {
                override fun getAvailableCommands(): Player.Commands {
                    return super.getAvailableCommands().buildUpon()
                        .add(COMMAND_SEEK_TO_NEXT)
                        .add(COMMAND_SEEK_TO_NEXT_MEDIA_ITEM)
                        .add(COMMAND_SEEK_TO_PREVIOUS)
                        .add(COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM)
                        .build()
                }

                override fun isCommandAvailable(command: Int): Boolean {
                    return when (command) {
                        COMMAND_SEEK_TO_NEXT, COMMAND_SEEK_TO_NEXT_MEDIA_ITEM,
                        COMMAND_SEEK_TO_PREVIOUS, COMMAND_SEEK_TO_PREVIOUS_MEDIA_ITEM -> true
                        else -> super.isCommandAvailable(command)
                    }
                }

                override fun seekToNext() {
                    FileLog.d("ForwardingPlayer.seekToNext()")
                    eventCallback?.onMediaAction("media-next", null)
                }

                override fun seekToNextMediaItem() {
                    FileLog.d("ForwardingPlayer.seekToNextMediaItem()")
                    eventCallback?.onMediaAction("media-next", null)
                }

                override fun seekToPrevious() {
                    FileLog.d("ForwardingPlayer.seekToPrevious()")
                    eventCallback?.onMediaAction("media-previous", null)
                }

                override fun seekToPreviousMediaItem() {
                    FileLog.d("ForwardingPlayer.seekToPreviousMediaItem()")
                    eventCallback?.onMediaAction("media-previous", null)
                }
            }

            mediaSession = MediaSession.Builder(this, wrapper).build()
            FileLog.d("MediaSession created")

            handler.postDelayed(positionTick, POSITION_INTERVAL_MS)

            instance = this
            synchronized(readyCallbacks) {
                FileLog.d("Flushing ${readyCallbacks.size} ready callbacks")
                readyCallbacks.forEach { it(this) }
                readyCallbacks.clear()
            }

            FileLog.d("PlaybackService.onCreate() complete — service ready")
        } catch (e: Exception) {
            FileLog.e("PlaybackService.onCreate() FAILED", e)
        }
    }

    override fun onGetSession(controllerInfo: MediaSession.ControllerInfo): MediaSession? {
        FileLog.d("onGetSession() called by ${controllerInfo.packageName}")
        return mediaSession
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        val player = exoPlayer
        val shouldStop = player == null || !player.playWhenReady || player.mediaItemCount == 0
        FileLog.d("onTaskRemoved: shouldStop=$shouldStop (playWhenReady=${player?.playWhenReady}, items=${player?.mediaItemCount})")
        if (shouldStop) {
            stopSelf()
        }
    }

    override fun onDestroy() {
        FileLog.d("PlaybackService.onDestroy()")
        instance = null
        handler.removeCallbacks(positionTick)
        mediaSession?.run {
            player.release()
            release()
        }
        mediaSession = null
        exoPlayer = null
        super.onDestroy()
    }

    // ── Public API (called by MediaSessionPlugin) ────────────────────────────

    fun loadTrack(url: String, title: String, artist: String, album: String?, artworkUri: String?) {
        FileLog.d("loadTrack: title=$title, artist=$artist, album=$album")
        FileLog.d("loadTrack: url=$url")
        FileLog.d("loadTrack: artworkUri=$artworkUri")

        val metadata = MediaMetadata.Builder()
            .setTitle(title)
            .setArtist(artist)
            .apply {
                if (album != null) setAlbumTitle(album)
                if (artworkUri != null) setArtworkUri(android.net.Uri.parse(artworkUri))
            }
            .build()

        val item = MediaItem.Builder()
            .setUri(url)
            .setMediaMetadata(metadata)
            .build()

        val p = exoPlayer
        if (p == null) {
            FileLog.e("loadTrack: exoPlayer is NULL — cannot load")
            return
        }

        FileLog.d("loadTrack: setting media item and preparing")
        p.setMediaItem(item)
        p.prepare()
        p.playWhenReady = true
        FileLog.d("loadTrack: prepare() called, playWhenReady=true, playerState=${p.playbackState}")
    }

    fun play() {
        FileLog.d("play() — current state: ${exoPlayer?.playbackState}, isPlaying=${exoPlayer?.isPlaying}")
        exoPlayer?.play()
    }

    fun pause() {
        FileLog.d("pause() — current state: ${exoPlayer?.playbackState}, isPlaying=${exoPlayer?.isPlaying}")
        exoPlayer?.pause()
    }

    fun seekTo(positionMs: Long) {
        FileLog.d("seekTo($positionMs)")
        exoPlayer?.seekTo(positionMs)
    }

    fun stopPlayback() {
        FileLog.d("stopPlayback()")
        exoPlayer?.stop()
        exoPlayer?.clearMediaItems()
    }
}
