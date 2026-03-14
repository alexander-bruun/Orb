package com.orb.app

import android.Manifest
import android.content.ComponentName
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.provider.Settings
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
        nativeInit(this.javaClass.classLoader, deviceId)

        requestNotificationPermission()
        startMediaService()
        connectMediaController()
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
}
