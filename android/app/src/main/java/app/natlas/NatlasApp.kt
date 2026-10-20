package app.natlas

import android.app.Application

// Process-wide state: the engine keeps running while the app's screen is recreated.
class NatlasApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Refresh.scheduleMidnight(this) // widgets switch to the new day without opening the app
        app.natlas.health.HealthSync.schedule(this) // the morning health sync (if turned on)
    }
}
