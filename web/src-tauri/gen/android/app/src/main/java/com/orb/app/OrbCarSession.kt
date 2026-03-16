package com.orb.app

import android.content.Intent
import androidx.car.app.Screen
import androidx.car.app.Session

class OrbCarSession : Session() {
    override fun onCreateScreen(intent: Intent): Screen {
        return MainScreen(carContext)
    }
}
