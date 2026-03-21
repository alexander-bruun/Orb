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

class MainScreen(carContext: CarContext) : Screen(carContext) {

    private var activeTabId = "HOME"
    private var items: List<Any> = emptyList()
    private var isLoading = true
    private var isOffline = false
    private val ioExecutor = Executors.newSingleThreadExecutor()

    // Cache loaded bitmaps so we don't re-fetch on every invalidate
    private val artworkCache = mutableMapOf<String, Bitmap>()

    init {
        fetchData()
    }

    private fun fetchData() {
        isLoading = true
        invalidate()
        ioExecutor.execute {
            try {
                var retry = 10
                while (MediaService.instance == null && retry > 0) {
                    Thread.sleep(500)
                    retry--
                }

                val svc = MediaService.instance
                val apiClient = svc?.apiClient

                if (apiClient != null) {
                    val reachable = try { apiClient.isReachable() } catch (e: Exception) { false }
                    if (reachable) {
                        isOffline = false
                        if (activeTabId == "OFFLINE") activeTabId = "HOME"

                        val fetched: List<Any> = when (activeTabId) {
                            "HOME"      -> apiClient.recentlyAddedAlbums(30)
                            "ALBUMS"    -> apiClient.albums(100, 0)
                            "PLAYLISTS" -> apiClient.playlists()
                            "ARTISTS"   -> apiClient.artists(100, 0)
                            else        -> apiClient.recentlyAddedAlbums(30)
                        }
                        items = fetched
                        preloadArtwork(fetched, apiClient)
                    } else {
                        isOffline = true
                        activeTabId = "OFFLINE"
                        items = svc.getDownloadMetadata()
                    }
                } else {
                    isOffline = true
                    activeTabId = "OFFLINE"
                    items = svc?.getDownloadMetadata() ?: emptyList()
                }

                isLoading = false
                invalidate()
            } catch (e: Exception) {
                Log.e("MainScreen", "Fetch failed", e)
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
        val tabBuilder = TabTemplate.Builder(object : TabTemplate.TabCallback {
            override fun onTabSelected(tabId: String) {
                if (tabId != activeTabId) {
                    activeTabId = tabId
                    artworkCache.clear()
                    fetchData()
                }
            }
        })

        if (isOffline) {
            tabBuilder.addTab(buildTab("Downloads", "OFFLINE", R.drawable.ic_check))
            tabBuilder.setActiveTabContentId("OFFLINE")
        } else {
            tabBuilder.addTab(buildTab("Home",      "HOME",      R.drawable.ic_play_arrow))
            tabBuilder.addTab(buildTab("Albums",    "ALBUMS",    R.drawable.ic_notification))
            tabBuilder.addTab(buildTab("Playlists", "PLAYLISTS", R.drawable.ic_plus))
            tabBuilder.addTab(buildTab("Artists",   "ARTISTS",   R.drawable.ic_person))
            tabBuilder.setActiveTabContentId(activeTabId)
        }

        tabBuilder.setTabContents(createTabContent())
        tabBuilder.setHeaderAction(Action.APP_ICON)

        return tabBuilder.build()
    }

    private fun buildTab(title: String, contentId: String, iconRes: Int): Tab {
        return Tab.Builder()
            .setTitle(title)
            .setContentId(contentId)
            .setIcon(CarIcon.Builder(IconCompat.createWithResource(carContext, iconRes)).build())
            .build()
    }

    private fun createTabContent(): TabContents {
        if (isLoading) {
            return TabContents.Builder(
                GridTemplate.Builder().setLoading(true).build()
            ).build()
        }

        val gridBuilder = ItemList.Builder()
        if (items.isEmpty()) {
            gridBuilder.setNoItemsMessage(
                if (isOffline) "No downloaded music found" else "No items found"
            )
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

                    is MediaService.DownloadMeta -> GridItem.Builder()
                        .setTitle(item.title)
                        .setText(item.artistName ?: "Downloaded")
                        .setImage(placeholderIcon())
                        .setOnClickListener { onOfflineTrackSelected(item) }
                        .build()

                    else -> null
                }
                gridItem?.let { gridBuilder.addItem(it) }
            }
        }

        return TabContents.Builder(
            GridTemplate.Builder().setSingleList(gridBuilder.build()).build()
        ).build()
    }

    private fun artworkOrPlaceholder(cacheKey: String): CarIcon {
        val bitmap = artworkCache[cacheKey]
        return if (bitmap != null) {
            CarIcon.Builder(IconCompat.createWithBitmap(bitmap)).build()
        } else {
            placeholderIcon()
        }
    }

    private fun placeholderIcon(): CarIcon =
        CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_notification)).build()

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

    private fun onOfflineTrackSelected(track: MediaService.DownloadMeta) {
        val svc = MediaService.instance ?: return
        val dir = java.io.File(svc.filesDir, "offline_audio")
        val file = java.io.File(dir, track.trackId)
        if (file.exists()) {
            svc.runOnUiThread {
                svc.handlePlay(file.absolutePath, track.title, track.artistName, null)
            }
        }
    }
}
