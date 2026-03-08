package com.plugin.mediasession

import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin

@InvokeArg
class LoadTrackArgs {
    lateinit var url: String
    lateinit var title: String
    lateinit var artist: String
    var album: String? = null
    var artwork: String? = null
}

@InvokeArg
class SeekArgs {
    var position_ms: Long = 0
}

@InvokeArg
class WriteLogArgs {
    lateinit var message: String
}

@TauriPlugin
class MediaSessionPlugin(private val activity: Activity) : Plugin(activity) {

    private var callbackRegistered = false
    private val handler = Handler(Looper.getMainLooper())

    override fun load(webView: android.webkit.WebView) {
        super.load(webView)
        FileLog.init(activity)
        FileLog.d("MediaSessionPlugin.load() called")
    }

    private fun ensureCallback() {
        if (callbackRegistered) return
        callbackRegistered = true
        FileLog.d("ensureCallback: registering event callback")

        PlaybackService.eventCallback = object : PlaybackService.EventCallback {
            override fun onPlaybackStateChanged(state: String, positionMs: Long) {
                FileLog.d("→JS playback_state: state=$state, pos=$positionMs")
                activity.runOnUiThread {
                    trigger("playback_state", JSObject().apply {
                        put("state", state)
                        put("position_ms", positionMs)
                    })
                }
            }

            override fun onPositionUpdate(positionMs: Long, durationMs: Long) {
                // Don't log every 500ms tick — too noisy
                activity.runOnUiThread {
                    trigger("position_update", JSObject().apply {
                        put("position_ms", positionMs)
                        put("duration_ms", durationMs)
                    })
                }
            }

            override fun onMediaAction(action: String, seekPos: Long?) {
                FileLog.d("→JS mediaAction: action=$action, seekPos=$seekPos")
                activity.runOnUiThread {
                    trigger("mediaAction", JSObject().apply {
                        put("action", action)
                        if (seekPos != null) put("seekPos", seekPos)
                    })
                }
            }

            override fun onTrackEnded() {
                FileLog.d("→JS track_ended")
                activity.runOnUiThread {
                    trigger("track_ended", JSObject())
                }
            }
        }
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val granted = activity.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) ==
                PackageManager.PERMISSION_GRANTED
            FileLog.d("POST_NOTIFICATIONS permission: granted=$granted")
            if (!granted) {
                activity.requestPermissions(
                    arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 1001
                )
            }
        }
    }

    private fun startService() {
        ensureCallback()
        if (PlaybackService.instance != null) {
            FileLog.d("startService: service already running")
            return
        }
        requestNotificationPermission()
        FileLog.d("startService: starting PlaybackService via startService()")
        try {
            val intent = Intent(activity, PlaybackService::class.java)
            activity.startService(intent)
            FileLog.d("startService: startService() returned successfully")
        } catch (e: Exception) {
            FileLog.e("startService: FAILED", e)
        }
    }

    // ── Commands ─────────────────────────────────────────────────────────────

    @Command
    fun initialize_player(invoke: Invoke) {
        FileLog.d("CMD initialize_player")
        startService()
        invoke.resolve()
    }

    @Command
    fun load_track(invoke: Invoke) {
        FileLog.d("CMD load_track — parsing args")
        try {
            val args = invoke.parseArgs(LoadTrackArgs::class.java)
            FileLog.d("CMD load_track: title=${args.title}, url=${args.url.take(120)}")
            startService()
            PlaybackService.whenReady { service ->
                handler.post {
                    FileLog.d("CMD load_track: whenReady fired, calling service.loadTrack()")
                    service.loadTrack(args.url, args.title, args.artist, args.album, args.artwork)
                }
            }
        } catch (e: Exception) {
            FileLog.e("CMD load_track: FAILED", e)
            invoke.reject(e.message ?: "load_track failed")
            return
        }
        invoke.resolve()
    }

    @Command
    fun play(invoke: Invoke) {
        FileLog.d("CMD play — instance=${PlaybackService.instance != null}")
        PlaybackService.instance?.play()
        invoke.resolve()
    }

    @Command
    fun pause(invoke: Invoke) {
        FileLog.d("CMD pause — instance=${PlaybackService.instance != null}")
        PlaybackService.instance?.pause()
        invoke.resolve()
    }

    @Command
    fun next_track(invoke: Invoke) {
        FileLog.d("CMD next_track (no-op)")
        invoke.resolve()
    }

    @Command
    fun previous_track(invoke: Invoke) {
        FileLog.d("CMD previous_track (no-op)")
        invoke.resolve()
    }

    @Command
    fun seek(invoke: Invoke) {
        val args = invoke.parseArgs(SeekArgs::class.java)
        FileLog.d("CMD seek: position_ms=${args.position_ms}")
        PlaybackService.instance?.seekTo(args.position_ms)
        invoke.resolve()
    }

    @Command
    fun stop(invoke: Invoke) {
        FileLog.d("CMD stop")
        PlaybackService.instance?.stopPlayback()
        val intent = Intent(activity, PlaybackService::class.java)
        activity.stopService(intent)
        invoke.resolve()
    }

    @Command
    fun write_log(invoke: Invoke) {
        try {
            val args = invoke.parseArgs(WriteLogArgs::class.java)
            FileLog.d("JS: ${args.message}")
        } catch (e: Exception) {
            FileLog.e("write_log parse error", e)
        }
        invoke.resolve()
    }

    @Command
    fun get_log(invoke: Invoke) {
        try {
            val dir = activity.getExternalFilesDir(null) ?: activity.filesDir
            val file = java.io.File(dir, "mediasession.log")
            if (file.exists()) {
                val content = file.readText()
                invoke.resolve(JSObject().apply { put("value", content) })
            } else {
                invoke.resolve(JSObject().apply { put("value", "(no log file found at ${file.absolutePath})") })
            }
        } catch (e: Exception) {
            invoke.resolve(JSObject().apply { put("value", "error reading log: ${e.message}") })
        }
    }

    override fun onDestroy() {
        FileLog.d("MediaSessionPlugin.onDestroy()")
        PlaybackService.eventCallback = null
        callbackRegistered = false
    }
}
