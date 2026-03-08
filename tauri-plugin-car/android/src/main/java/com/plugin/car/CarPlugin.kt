package com.plugin.car

import android.app.Activity
import android.content.ComponentName
import android.content.Intent
import android.os.Build
import android.support.v4.media.MediaBrowserCompat
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import org.json.JSONArray
import org.json.JSONObject

@InvokeArg
class SetNowPlayingArgs {
    lateinit var id: String
    lateinit var title: String
    var artist: String? = null
    var album: String? = null
    var duration_ms: Long? = null
    var artwork_url: String? = null
}

@InvokeArg
class SetMediaRootArgs {
    lateinit var items: String // JSON array string
}

@InvokeArg
class SetPlaybackStateArgs {
    var playing: Boolean = false
    var position_ms: Long = 0
}

@TauriPlugin
class CarPlugin(private val activity: Activity) : Plugin(activity) {

    private var mediaBrowser: MediaBrowserCompat? = null
    private var browserService: CarMediaBrowserService? = null

    override fun load(webView: android.webkit.WebView) {
        super.load(webView)

        // Set up action callback so service events reach JS.
        CarMediaBrowserService.actionCallback = { action, payload ->
            val data = JSObject()
            data.put("action", action)
            if (payload != null) data.put("payload", payload)
            trigger("carAction", data)
        }

        // Connect a MediaBrowser to our service so it starts.
        val component = ComponentName(activity, CarMediaBrowserService::class.java)
        mediaBrowser = MediaBrowserCompat(
            activity, component,
            object : MediaBrowserCompat.ConnectionCallback() {
                override fun onConnected() {
                    val data = JSObject()
                    data.put("connected", true)
                    trigger("carConnection", data)
                }
                override fun onConnectionSuspended() {
                    val data = JSObject()
                    data.put("connected", false)
                    trigger("carConnection", data)
                }
            },
            null
        )
        mediaBrowser?.connect()
    }

    @Command
    fun set_now_playing(invoke: Invoke) {
        val args = invoke.parseArgs(SetNowPlayingArgs::class.java)

        CarMediaBrowserService.currentTrack = TrackData(
            id = args.id,
            title = args.title,
            artist = args.artist,
            album = args.album,
            durationMs = args.duration_ms,
            artworkUrl = args.artwork_url
        )

        // If the service is running, refresh now-playing immediately.
        // The service is started by Android Auto when it connects.
        invoke.resolve()
    }

    @Command
    fun set_media_root(invoke: Invoke) {
        val args = invoke.parseArgs(SetMediaRootArgs::class.java)

        try {
            val jsonArray = JSONArray(args.items)
            CarMediaBrowserService.mediaItems = parseMediaItems(jsonArray)
        } catch (e: Exception) {
            invoke.reject("Failed to parse media items: ${e.message}")
            return
        }

        invoke.resolve()
    }

    @Command
    fun set_playback_state(invoke: Invoke) {
        val args = invoke.parseArgs(SetPlaybackStateArgs::class.java)

        CarMediaBrowserService.isPlaying = args.playing
        CarMediaBrowserService.positionMs = args.position_ms

        invoke.resolve()
    }

    @Command
    fun on_car_connected(invoke: Invoke) {
        // No-op: connection events are pushed via the carConnection event.
        invoke.resolve()
    }

    private fun parseMediaItems(jsonArray: JSONArray): List<MediaItemData> {
        val items = mutableListOf<MediaItemData>()
        for (i in 0 until jsonArray.length()) {
            val obj = jsonArray.getJSONObject(i)
            val children = if (obj.has("children") && !obj.isNull("children")) {
                parseMediaItems(obj.getJSONArray("children"))
            } else null

            items.add(
                MediaItemData(
                    id = obj.getString("id"),
                    title = obj.getString("title"),
                    subtitle = obj.optString("subtitle", null),
                    playable = obj.optBoolean("playable", false),
                    artworkUrl = obj.optString("artwork_url", null),
                    children = children
                )
            )
        }
        return items
    }

    override fun onDestroy() {
        mediaBrowser?.disconnect()
        CarMediaBrowserService.actionCallback = null
        super.onDestroy()
    }
}
