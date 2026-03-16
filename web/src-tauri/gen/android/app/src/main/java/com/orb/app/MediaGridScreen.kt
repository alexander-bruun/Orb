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

    init {
        fetchData()
    }

    private fun fetchData() {
        ioExecutor.execute {
            try {
                // Wait for instance to be ready
                var retry = 5
                while (MediaService.instance == null && retry > 0) {
                    Thread.sleep(500)
                    retry--
                }
                
                val apiClient = MediaService.instance?.apiClient

                if (apiClient != null) {
                    items = when (fetchMode) {
                        "RECENTLY_ADDED" -> apiClient.recentlyAddedAlbums(30)
                        "ALBUMS" -> apiClient.albums(100, 0)
                        "PLAYLISTS" -> apiClient.playlists()
                        "ARTISTS" -> apiClient.artists(100, 0)
                        else -> emptyList()
                    }
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

    override fun onGetTemplate(): Template {
        if (isLoading) {
            return GridTemplate.Builder()
                .setTitle(title)
                .setLoading(true)
                .setHeaderAction(Action.BACK)
                .build()
        }

        val gridBuilder = ItemList.Builder()
        if (items.isEmpty()) {
            gridBuilder.setNoItemsMessage("No items found")
        } else {
            for (item in items) {
                val gridItem = when (item) {
                    is OrbApiClient.BrowseAlbum -> {
                        GridItem.Builder()
                            .setTitle(item.title)
                            .setText(item.artistName ?: "")
                            .setImage(createPlaceholderIcon())
                            .setOnClickListener { onAlbumSelected(item) }
                            .build()
                    }
                    is OrbApiClient.BrowsePlaylist -> {
                        GridItem.Builder()
                            .setTitle(item.name)
                            .setImage(createPlaceholderIcon())
                            .setOnClickListener { onPlaylistSelected(item) }
                            .build()
                    }
                    is OrbApiClient.BrowseArtist -> {
                        GridItem.Builder()
                            .setTitle(item.name)
                            .setImage(createPlaceholderIcon())
                            .setOnClickListener { onArtistSelected(item) }
                            .build()
                    }
                    else -> null
                }
                gridItem?.let { gridBuilder.addItem(it) }
            }
        }

        return GridTemplate.Builder()
            .setTitle(title)
            .setSingleList(gridBuilder.build())
            .setHeaderAction(Action.BACK)
            .build()
    }

    private fun createPlaceholderIcon(): CarIcon {
        return CarIcon.Builder(
            IconCompat.createWithResource(carContext, R.drawable.ic_notification)
        ).build()
    }

    private fun onAlbumSelected(album: OrbApiClient.BrowseAlbum) {
        screenManager.push(TrackListScreen(carContext, album.title, "ALBUM", album.id))
    }

    private fun onPlaylistSelected(playlist: OrbApiClient.BrowsePlaylist) {
        screenManager.push(TrackListScreen(carContext, playlist.name, "PLAYLIST", playlist.id))
    }

    private fun onArtistSelected(artist: OrbApiClient.BrowseArtist) {
        screenManager.push(MediaGridScreen(carContext, artist.name, "ARTIST_ALBUMS:" + artist.id))
    }
}
