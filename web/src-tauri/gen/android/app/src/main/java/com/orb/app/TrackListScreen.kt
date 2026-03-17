package com.orb.app

import android.util.Log
import androidx.car.app.CarContext
import androidx.car.app.Screen
import androidx.car.app.model.*
import androidx.car.app.model.Action
import java.util.concurrent.Executors

class TrackListScreen(
    carContext: CarContext,
    private val title: String,
    private val type: String,
    private val id: String
) : Screen(carContext) {

    private var tracks: List<OrbApiClient.BrowseTrack> = emptyList()
    private var isLoading = true
    private val ioExecutor = Executors.newSingleThreadExecutor()

    init {
        fetchTracks()
    }

    private fun fetchTracks() {
        ioExecutor.execute {
            try {
                val apiClient = MediaService.instance?.apiClient

                if (apiClient != null) {
                    tracks = when (type) {
                        "ALBUM" -> apiClient.albumDetail(id)?.tracks ?: emptyList()
                        "PLAYLIST" -> apiClient.playlistDetail(id)?.tracks ?: emptyList()
                        else -> emptyList()
                    }
                }
                isLoading = false
                invalidate()
            } catch (e: Exception) {
                Log.e("TrackListScreen", "Fetch failed", e)
                isLoading = false
                invalidate()
            }
        }
    }

    override fun onGetTemplate(): Template {
        val header = Header.Builder()
            .setTitle(title)
            .setStartHeaderAction(Action.BACK)
            .build()

        if (isLoading) {
            return ListTemplate.Builder()
                .setLoading(true)
                .setHeader(header)
                .build()
        }

        val listBuilder = ItemList.Builder()
        if (tracks.isEmpty()) {
            listBuilder.setNoItemsMessage("No tracks found")
        } else {
            for (track in tracks) {
                val row = Row.Builder()
                    .setTitle(track.title)
                    .addText(track.artistName ?: "")
                    .setOnClickListener { onTrackSelected(track) }
                    .build()
                listBuilder.addItem(row)
            }
        }

        return ListTemplate.Builder()
            .setSingleList(listBuilder.build())
            .setHeader(header)
            .build()
    }

    private fun onTrackSelected(track: OrbApiClient.BrowseTrack) {
        val svc = MediaService.instance ?: return
        val apiClient = svc.apiClient ?: return

        val url = apiClient.streamUrl(track.id)
        val coverUrl = track.albumId?.let { apiClient.coverUrl(it) }
        
        svc.runOnUiThread {
            svc.handlePlay(url, track.title, track.artistName, coverUrl)
        }
    }
}
