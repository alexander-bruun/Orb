package com.orb.app

import android.util.Log
import androidx.car.app.CarContext
import androidx.car.app.Screen
import androidx.car.app.model.*
import androidx.car.app.model.Action
import androidx.core.graphics.drawable.IconCompat
import java.util.concurrent.Executors

class MainScreen(carContext: CarContext) : Screen(carContext) {

    private var activeTabId = "HOME"
    private var items: List<Any> = emptyList()
    private var isLoading = true
    private var isOffline = false
    private val ioExecutor = Executors.newSingleThreadExecutor()

    init {
        fetchData()
    }

    private fun fetchData() {
        isLoading = true
        invalidate()
        ioExecutor.execute {
            try {
                // Wait for instance to be ready
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
                        // If we were forced into OFFLINE tab, switch back to HOME
                        if (activeTabId == "OFFLINE") {
                            activeTabId = "HOME"
                        }
                        
                        items = when (activeTabId) {
                            "HOME" -> apiClient.recentlyAddedAlbums(30)
                            "ALBUMS" -> apiClient.albums(100, 0)
                            "PLAYLISTS" -> apiClient.playlists()
                            else -> apiClient.recentlyAddedAlbums(30)
                        }
                    } else {
                        isOffline = true
                        activeTabId = "OFFLINE"
                        items = svc.getDownloadMetadata()
                    }
                } else {
                    // No API client yet, try to show offline if available
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

    override fun onGetTemplate(): Template {
        val tabBuilder = TabTemplate.Builder(object : TabTemplate.TabCallback {
            override fun onTabSelected(tabId: String) {
                if (tabId != activeTabId) {
                    activeTabId = tabId
                    fetchData()
                }
            }
        })

        if (isOffline) {
            tabBuilder.addTab(
                Tab.Builder()
                    .setTitle("Downloads")
                    .setContentId("OFFLINE")
                    .setIcon(CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_check)).build())
                    .build()
            )
            tabBuilder.setActiveTabContentId("OFFLINE")
        } else {
            tabBuilder.addTab(
                Tab.Builder()
                    .setTitle("Home")
                    .setContentId("HOME")
                    .setIcon(CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_play_arrow)).build())
                    .build()
            )
            tabBuilder.addTab(
                Tab.Builder()
                    .setTitle("Albums")
                    .setContentId("ALBUMS")
                    .setIcon(CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_notification)).build())
                    .build()
            )
            tabBuilder.addTab(
                Tab.Builder()
                    .setTitle("Playlists")
                    .setContentId("PLAYLISTS")
                    .setIcon(CarIcon.Builder(IconCompat.createWithResource(carContext, R.drawable.ic_plus)).build())
                    .build()
            )
            tabBuilder.setActiveTabContentId(activeTabId)
        }

        tabBuilder.setTabContents(createTabContent())
        tabBuilder.setHeaderAction(Action.APP_ICON)

        return tabBuilder.build()
    }

    private fun createTabContent(): TabContents {
        if (isLoading) {
            // Note: GridTemplate or ListTemplate within TabContents might need special handling
            // based on the library version. Usually TabContents takes a Template.
            val gridTemplate = GridTemplate.Builder()
                .setLoading(true)
                .build()
            return TabContents.Builder(gridTemplate).build()
        }

        val gridBuilder = ItemList.Builder()
        if (items.isEmpty()) {
            gridBuilder.setNoItemsMessage(if (isOffline) "No downloaded music found" else "No items found")
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
                    is MediaService.DownloadMeta -> {
                        GridItem.Builder()
                            .setTitle(item.title)
                            .setText(item.artistName ?: "Downloaded")
                            .setImage(createPlaceholderIcon())
                            .setOnClickListener { onOfflineTrackSelected(item) }
                            .build()
                    }
                    else -> null
                }
                gridItem?.let { gridBuilder.addItem(it) }
            }
        }

        val gridTemplate = GridTemplate.Builder()
            .setSingleList(gridBuilder.build())
            .build()
            
        return TabContents.Builder(gridTemplate).build()
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

    private fun onOfflineTrackSelected(track: MediaService.DownloadMeta) {
        val svc = MediaService.instance ?: return
        // Use local file path for offline playback
        val dir = java.io.File(svc.filesDir, "offline_audio")
        val file = java.io.File(dir, track.trackId)
        if (file.exists()) {
            val url = file.absolutePath
            svc.runOnUiThread {
                svc.handlePlay(url, track.title, track.artistName, null)
            }
        }
    }
}
