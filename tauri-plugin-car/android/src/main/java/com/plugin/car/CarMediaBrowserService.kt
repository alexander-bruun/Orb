package com.plugin.car

import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.os.Bundle
import android.support.v4.media.MediaBrowserCompat
import android.support.v4.media.MediaDescriptionCompat
import android.support.v4.media.MediaMetadataCompat
import android.support.v4.media.session.MediaSessionCompat
import android.support.v4.media.session.PlaybackStateCompat
import androidx.media.MediaBrowserServiceCompat
import java.net.URL

class CarMediaBrowserService : MediaBrowserServiceCompat() {

    companion object {
        private const val ROOT_ID = "__ROOT__"

        // Shared mutable state set by CarPlugin via static setters.
        @Volatile var mediaItems: List<MediaItemData> = emptyList()
        @Volatile var currentTrack: TrackData? = null
        @Volatile var isPlaying: Boolean = false
        @Volatile var positionMs: Long = 0L

        // Callback to forward car actions back to the Tauri plugin.
        var actionCallback: ((String, String?) -> Unit)? = null
    }

    private lateinit var mediaSession: MediaSessionCompat

    override fun onCreate() {
        super.onCreate()

        mediaSession = MediaSessionCompat(this, "OrbCarSession").apply {
            setCallback(object : MediaSessionCompat.Callback() {
                override fun onPlay() {
                    actionCallback?.invoke("car-play", null)
                }
                override fun onPause() {
                    actionCallback?.invoke("car-pause", null)
                }
                override fun onSkipToNext() {
                    actionCallback?.invoke("car-next", null)
                }
                override fun onSkipToPrevious() {
                    actionCallback?.invoke("car-previous", null)
                }
                override fun onStop() {
                    actionCallback?.invoke("car-stop", null)
                }
                override fun onPlayFromMediaId(mediaId: String?, extras: Bundle?) {
                    actionCallback?.invoke("car-play-item", mediaId)
                }
                override fun onSeekTo(pos: Long) {
                    actionCallback?.invoke("car-seekto", pos.toString())
                }
            })
            isActive = true
        }

        sessionToken = mediaSession.sessionToken
        syncPlaybackState()
    }

    override fun onGetRoot(
        clientPackageName: String,
        clientUid: Int,
        rootHints: Bundle?
    ): BrowserRoot {
        return BrowserRoot(ROOT_ID, null)
    }

    override fun onLoadChildren(
        parentId: String,
        result: Result<MutableList<MediaBrowserCompat.MediaItem>>
    ) {
        if (parentId == ROOT_ID) {
            val items = mediaItems.map { item ->
                val desc = MediaDescriptionCompat.Builder()
                    .setMediaId(item.id)
                    .setTitle(item.title)
                    .setSubtitle(item.subtitle)
                    .build()
                val flags = if (item.playable) {
                    MediaBrowserCompat.MediaItem.FLAG_PLAYABLE
                } else {
                    MediaBrowserCompat.MediaItem.FLAG_BROWSABLE
                }
                MediaBrowserCompat.MediaItem(desc, flags)
            }
            result.sendResult(items.toMutableList())
        } else {
            // Find the parent item and return its children.
            val children = findChildren(parentId, mediaItems)
            if (children != null) {
                val items = children.map { item ->
                    val desc = MediaDescriptionCompat.Builder()
                        .setMediaId(item.id)
                        .setTitle(item.title)
                        .setSubtitle(item.subtitle)
                        .build()
                    val flags = if (item.playable) {
                        MediaBrowserCompat.MediaItem.FLAG_PLAYABLE
                    } else {
                        MediaBrowserCompat.MediaItem.FLAG_BROWSABLE
                    }
                    MediaBrowserCompat.MediaItem(desc, flags)
                }
                result.sendResult(items.toMutableList())
            } else {
                result.sendResult(mutableListOf())
            }
        }
    }

    private fun findChildren(parentId: String, items: List<MediaItemData>): List<MediaItemData>? {
        for (item in items) {
            if (item.id == parentId) return item.children ?: emptyList()
            val children = item.children
            if (children != null) {
                val found = findChildren(parentId, children)
                if (found != null) return found
            }
        }
        return null
    }

    fun refreshNowPlaying() {
        val track = currentTrack ?: return

        val metaBuilder = MediaMetadataCompat.Builder()
            .putString(MediaMetadataCompat.METADATA_KEY_MEDIA_ID, track.id)
            .putString(MediaMetadataCompat.METADATA_KEY_TITLE, track.title)
            .putString(MediaMetadataCompat.METADATA_KEY_ARTIST, track.artist ?: "")
            .putString(MediaMetadataCompat.METADATA_KEY_ALBUM, track.album ?: "")

        track.durationMs?.let {
            metaBuilder.putLong(MediaMetadataCompat.METADATA_KEY_DURATION, it)
        }

        mediaSession.setMetadata(metaBuilder.build())

        // Load artwork in background.
        val artworkUrl = track.artworkUrl
        if (artworkUrl != null) {
            Thread {
                try {
                    val bitmap = BitmapFactory.decodeStream(URL(artworkUrl).openStream())
                    metaBuilder.putBitmap(MediaMetadataCompat.METADATA_KEY_ALBUM_ART, bitmap)
                    mediaSession.setMetadata(metaBuilder.build())
                } catch (_: Exception) {}
            }.start()
        }

        syncPlaybackState()
    }

    fun syncPlaybackState() {
        val state = if (isPlaying) {
            PlaybackStateCompat.STATE_PLAYING
        } else {
            PlaybackStateCompat.STATE_PAUSED
        }

        mediaSession.setPlaybackState(
            PlaybackStateCompat.Builder()
                .setActions(
                    PlaybackStateCompat.ACTION_PLAY or
                    PlaybackStateCompat.ACTION_PAUSE or
                    PlaybackStateCompat.ACTION_SKIP_TO_NEXT or
                    PlaybackStateCompat.ACTION_SKIP_TO_PREVIOUS or
                    PlaybackStateCompat.ACTION_SEEK_TO or
                    PlaybackStateCompat.ACTION_STOP or
                    PlaybackStateCompat.ACTION_PLAY_PAUSE or
                    PlaybackStateCompat.ACTION_PLAY_FROM_MEDIA_ID
                )
                .setState(state, positionMs, if (isPlaying) 1f else 0f)
                .build()
        )
    }

    fun refreshMediaTree() {
        notifyChildrenChanged(ROOT_ID)
    }

    override fun onDestroy() {
        mediaSession.isActive = false
        mediaSession.release()
        super.onDestroy()
    }
}

data class TrackData(
    val id: String,
    val title: String,
    val artist: String?,
    val album: String?,
    val durationMs: Long?,
    val artworkUrl: String?
)

data class MediaItemData(
    val id: String,
    val title: String,
    val subtitle: String?,
    val playable: Boolean,
    val artworkUrl: String?,
    val children: List<MediaItemData>?
)
