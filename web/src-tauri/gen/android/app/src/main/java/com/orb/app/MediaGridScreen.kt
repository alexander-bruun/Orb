package com.orb.app

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.Log
import androidx.car.app.CarContext
import androidx.car.app.Screen
import androidx.car.app.model.*
import androidx.car.app.model.Action
import androidx.core.graphics.drawable.IconCompat
import java.net.URL
import java.util.concurrent.Executors

class MediaGridScreen(
    carContext: CarContext,
    private val title: String,
    private val fetchMode: String
) : Screen(carContext) {

    private var items: List<Any> = emptyList()
    private var isLoading = true
    private val ioExecutor = Executors.newSingleThreadExecutor()
    private val artworkCache = mutableMapOf<String, Bitmap>()

    init {
        fetchData()
    }

    private fun fetchData() {
        ioExecutor.execute {
            try {
                var retry = 5
                while (MediaService.instance == null && retry > 0) {
                    Thread.sleep(500)
                    retry--
                }

                val apiClient = MediaService.instance?.apiClient

                if (apiClient != null) {
                    val fetched: List<Any> = when {
                        fetchMode == "RECENTLY_ADDED"            -> apiClient.recentlyAddedAlbums(30)
                        fetchMode == "ALBUMS"                    -> apiClient.albums(100, 0)
                        fetchMode == "PLAYLISTS"                 -> apiClient.playlists()
                        fetchMode == "ARTISTS"                   -> apiClient.artists(100, 0)
                        fetchMode.startsWith("ARTIST_ALBUMS:")   -> {
                            val artistId = fetchMode.removePrefix("ARTIST_ALBUMS:")
                            apiClient.artistAlbums(artistId)
                        }
                        else -> emptyList()
                    }
                    items = fetched
                    preloadArtwork(fetched, apiClient)
                }
                isLoading = false
                invalidate()
            } catch (e: Exception) {
                Log.e("MediaGridScreen", "Fetch failed", e)
                isLoading = false
                invalidate()
            }
        }
    }

    private fun preloadArtwork(items: List<Any>, apiClient: OrbApiClient) {
        for (item in items) {
            val (key, url) = when (item) {
                is OrbApiClient.BrowseAlbum    -> item.id to apiClient.coverUrl(item.id)
                is OrbApiClient.BrowseArtist   -> item.id to apiClient.artistCoverUrl(item.id)
                is OrbApiClient.BrowsePlaylist -> item.id to apiClient.playlistCoverUrl(item.id)
                else -> continue
            }
            if (!artworkCache.containsKey(key)) {
                loadBitmap(url)?.let { artworkCache[key] = it }
            }
        }
    }

    override fun onGetTemplate(): Template {
        val header = Header.Builder()
            .setTitle(title)
            .setStartHeaderAction(Action.BACK)
            .build()

        if (isLoading) {
            return GridTemplate.Builder()
                .setLoading(true)
                .setHeader(header)
                .build()
        }

        val gridBuilder = ItemList.Builder()
        if (items.isEmpty()) {
            gridBuilder.setNoItemsMessage("No items found")
        } else {
            for (item in items) {
                val gridItem = when (item) {
                    is OrbApiClient.BrowseAlbum -> GridItem.Builder()
                        .setTitle(item.title)
                        .setText(item.artistName ?: "")
                        .setImage(artworkOrPlaceholder(item.id))
                        .setOnClickListener { onAlbumSelected(item) }
                        .build()

                    is OrbApiClient.BrowsePlaylist -> GridItem.Builder()
                        .setTitle(item.name)
                        .setText(item.description ?: "")
                        .setImage(artworkOrPlaceholder(item.id))
                        .setOnClickListener { onPlaylistSelected(item) }
                        .build()

                    is OrbApiClient.BrowseArtist -> GridItem.Builder()
                        .setTitle(item.name)
                        .setImage(artworkOrPlaceholder(item.id))
                        .setOnClickListener { onArtistSelected(item) }
                        .build()

                    else -> null
                }
                gridItem?.let { gridBuilder.addItem(it) }
            }
        }

        return GridTemplate.Builder()
            .setSingleList(gridBuilder.build())
            .setHeader(header)
            .build()
    }

    private fun artworkOrPlaceholder(cacheKey: String): CarIcon {
        val bitmap = artworkCache[cacheKey]
        return if (bitmap != null) {
            CarIcon.Builder(IconCompat.createWithBitmap(bitmap)).build()
        } else {
            CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_notification)).build()
        }
    }

    private fun loadBitmap(url: String): Bitmap? {
        return try {
            val conn = URL(url).openConnection()
            conn.connectTimeout = 4000
            conn.readTimeout = 4000
            BitmapFactory.decodeStream(conn.getInputStream())
        } catch (_: Exception) { null }
    }

    private fun onAlbumSelected(album: OrbApiClient.BrowseAlbum) {
        screenManager.push(TrackListScreen(carContext, album.title, "ALBUM", album.id))
    }

    private fun onPlaylistSelected(playlist: OrbApiClient.BrowsePlaylist) {
        screenManager.push(TrackListScreen(carContext, playlist.name, "PLAYLIST", playlist.id))
    }

    private fun onArtistSelected(artist: OrbApiClient.BrowseArtist) {
        screenManager.push(MediaGridScreen(carContext, artist.name, "ARTIST_ALBUMS:${artist.id}"))
    }
}
