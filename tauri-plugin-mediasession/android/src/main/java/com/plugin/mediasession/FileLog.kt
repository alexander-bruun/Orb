package com.plugin.mediasession

import android.content.Context
import android.util.Log
import java.io.File
import java.io.FileWriter
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Simple file logger that writes to the app's external files directory.
 *
 * Log file location: /sdcard/Android/data/<package>/files/mediasession.log
 *
 * Pull with:  adb shell cat /sdcard/Android/data/<package>/files/mediasession.log
 * Or:         adb pull /sdcard/Android/data/<package>/files/mediasession.log
 */
object FileLog {
    private const val TAG = "MediaSession"
    private const val FILE_NAME = "mediasession.log"
    private const val MAX_SIZE = 2 * 1024 * 1024 // 2 MB — rotate when exceeded

    private var file: File? = null
    private val dateFormat = SimpleDateFormat("HH:mm:ss.SSS", Locale.US)

    fun init(context: Context) {
        try {
            val dir = context.getExternalFilesDir(null) ?: context.filesDir
            file = File(dir, FILE_NAME)
            // Rotate if too large
            file?.let { f ->
                if (f.exists() && f.length() > MAX_SIZE) {
                    val old = File(dir, "$FILE_NAME.old")
                    old.delete()
                    f.renameTo(old)
                    file = File(dir, FILE_NAME)
                }
            }
            d("──────────────── session start ────────────────")
            d("log file: ${file?.absolutePath}")
        } catch (e: Exception) {
            Log.e(TAG, "FileLog.init failed", e)
        }
    }

    fun d(msg: String) {
        val ts = dateFormat.format(Date())
        val line = "$ts  $msg"
        Log.d(TAG, msg)
        try {
            file?.let { f ->
                FileWriter(f, true).use { it.appendLine(line) }
            }
        } catch (_: Exception) {}
    }

    fun e(msg: String, t: Throwable? = null) {
        val ts = dateFormat.format(Date())
        val err = if (t != null) "$msg — ${t::class.simpleName}: ${t.message}" else msg
        val line = "$ts  ERROR $err"
        Log.e(TAG, msg, t)
        try {
            file?.let { f ->
                FileWriter(f, true).use { w ->
                    w.appendLine(line)
                    t?.stackTrace?.take(8)?.forEach { frame ->
                        w.appendLine("$ts    at $frame")
                    }
                }
            }
        } catch (_: Exception) {}
    }
}
