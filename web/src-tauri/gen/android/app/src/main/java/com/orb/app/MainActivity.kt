package com.orb.app

import android.Manifest
import android.content.ComponentName
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.webkit.WebView
import androidx.activity.enableEdgeToEdge
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.media3.session.MediaController
import androidx.media3.session.SessionToken

class MainActivity : TauriActivity() {

    companion object {
        private const val NOTIFICATION_PERMISSION_CODE = 1001

        @JvmStatic
        external fun nativeInit(classLoader: ClassLoader, deviceId: String)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)

        val deviceId = Settings.Secure.getString(contentResolver, Settings.Secure.ANDROID_ID) ?: ""
        val classLoader = this.javaClass.classLoader ?: throw IllegalStateException("ClassLoader not found")
        nativeInit(classLoader, deviceId)

        requestNotificationPermission()
        startMediaService()
        connectMediaController()
    }

    override fun onPostResume() {
        super.onPostResume()
        disableWebViewZoom()
    }

    private fun requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED
            ) {
                ActivityCompat.requestPermissions(
                    this,
                    arrayOf(Manifest.permission.POST_NOTIFICATIONS),
                    NOTIFICATION_PERMISSION_CODE
                )
            }
        }
    }

    private fun startMediaService() {
        val intent = Intent(this, MediaService::class.java)
        startService(intent)
    }

    private fun connectMediaController() {
        val sessionToken = SessionToken(this, ComponentName(this, MediaService::class.java))
        MediaController.Builder(this, sessionToken).buildAsync()
    }

    private fun disableWebViewZoom() {
        // Disable pinch-to-zoom and built-in zoom controls on the WebView
        try {
            val webView = findWebView(window.decorView)
            if (webView != null) {
                val settings = webView.settings
                settings.setSupportZoom(false)
                settings.setBuiltInZoomControls(false)
                settings.setDisplayZoomControls(false)
            }
        } catch (e: Exception) {
            e.printStackTrace()
        }
    }

    private fun findWebView(view: android.view.View): WebView? {
        if (view is WebView) {
            return view
        }
        if (view is android.view.ViewGroup) {
            for (i in 0 until view.childCount) {
                val child = findWebView(view.getChildAt(i))
                if (child != null) {
                    return child
                }
            }
        }
        return null
    }
}
