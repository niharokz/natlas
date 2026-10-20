package app.natlas

import android.content.Intent
import android.net.Uri
import android.provider.Settings
import android.webkit.JavascriptInterface
import app.natlas.health.HealthConnect
import app.natlas.health.HealthSync
import org.json.JSONArray
import org.json.JSONObject
import java.text.DateFormat
import java.util.Date

// window.NatlasNative in the web app (web/app.js, "phone" section). Calls arrive
// on a background thread; anything touching views is posted to the UI thread.
class Bridge(private val act: MainActivity) {
    private val prefs = Prefs(act)

    @JavascriptInterface
    fun settings(): String = snapshot().toString()

    private fun snapshot(): JSONObject {
        val (ok, note) = Storage.check(act, prefs.folder)
        val plugins = JSONArray()
        val on = prefs.enabled
        for (id in prefs.order) plugins.put(JSONObject().put("id", id).put("title", Prefs.TITLES[id] ?: id).put("on", id in on))
        val last = prefs.lastRefresh
        return JSONObject()
            .put("folder", if (Storage.isDemo(act, prefs.folder)) "Sample data (in the app)" else prefs.folder)
            .put("folderOk", ok).put("folderNote", note).put("fileAccess", Storage.hasAccess())
            .put("plugins", plugins).put("start", prefs.start)
            .put("lock", prefs.lockMode).put("hasPin", prefs.pinHash.isNotEmpty()).put("pinBio", prefs.pinBio)
            .put("delay", prefs.lockDelay).put("secure", prefs.secure).put("animations", prefs.animations)
            .put("main", prefs.main).put("deviceSecure", Lock.deviceSecure(act)).put("biometric", Lock.biometricReady(act))
            .put("widgets", JSONObject().put("theme", prefs.widgetTheme).put("opacity", prefs.widgetOpacity)
                .put("accent", prefs.widgetAccent).put("privacy", prefs.widgetPrivacy)
                .put("last", if (last == 0L) "" else DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(last))))
            .put("health", health())
            .put("configDir", prefs.configDir).put("configNote", AppConfig.describe(act))
            .put("version", BuildConfigVersion.name(act))
            .put("engine", if (Engine.alive()) "running on port ${Engine.port}" else "stopped")
    }

    private fun health(): JSONObject {
        val ok = HealthSync.supported()
        val o = JSONObject().put("supported", ok).put("on", prefs.healthOn).put("time", prefs.healthTime)
            .put("note", prefs.healthNote).put("sources", prefs.healthSources).put("imported", prefs.healthImported).put("running", healthBusy)
            .put("last", if (prefs.healthLast == 0L) "" else DateFormat.getDateTimeInstance(DateFormat.MEDIUM, DateFormat.SHORT).format(Date(prefs.healthLast)))
        if (ok) o.put("granted", HealthConnect.grantedCount(act)).put("total", HealthConnect.READ.size)
            .put("background", HealthConnect.granted(act, HealthConnect.BACKGROUND)).put("history", HealthConnect.granted(act, HealthConnect.HISTORY))
        return o
    }

    @Volatile private var healthBusy = false

    // Runs a health sync off the UI thread and reports back to the page.
    private fun healthRun(work: () -> String) {
        if (healthBusy) return
        healthBusy = true
        act.emit("settings", snapshot())
        Thread {
            val text = try { work() } finally { healthBusy = false }
            act.emit("toast", JSONObject().put("text", text))
            act.emit("settings", snapshot())
        }.start()
    }

    @JavascriptInterface fun healthGrant() = act.runOnUiThread { act.requestHealth() }
    @JavascriptInterface fun healthSyncNow() = healthRun { HealthSync.runDaily(act.applicationContext) }
    @JavascriptInterface fun healthImport() = healthRun {
        HealthSync.importHistory(act.applicationContext) { step -> act.emit("health", JSONObject().put("progress", step)) }
    }

    // Health Connect's settings page for Natlas (to change or revoke access).
    @JavascriptInterface
    fun healthManage() = act.runOnUiThread {
        try {
            act.startActivity(Intent("android.health.connect.action.MANAGE_HEALTH_PERMISSIONS").putExtra(Intent.EXTRA_PACKAGE_NAME, act.packageName))
        } catch (_: Exception) { act.startActivity(Intent("android.health.connect.action.HEALTH_HOME_SETTINGS")) }
    }

    @JavascriptInterface
    fun set(key: String, value: String): String {
        var restart = false
        var widgets = false
        when (key) {
            "enabled" -> { prefs.enabled = value.split(",").toSet(); restart = true }
            "order" -> { prefs.order = value.split(","); restart = true }
            "start" -> prefs.start = value
            "lock" -> {
                if (value == "pin" && prefs.pinHash.isEmpty()) act.runOnUiThread { act.choosePin(thenMode = "pin") }
                else { prefs.lockMode = value; if (value != "off") Lock.unlocked() }
            }
            "pinBio" -> prefs.pinBio = value == "true"
            "delay" -> prefs.lockDelay = value.toIntOrNull() ?: 60
            "secure" -> { prefs.secure = value == "true"; act.runOnUiThread { act.applySecure() } }
            "animations" -> prefs.animations = value == "true"
            "main" -> { prefs.main = value == "true"; restart = true }
            "widgetTheme" -> { prefs.widgetTheme = value; widgets = true }
            "widgetOpacity" -> { prefs.widgetOpacity = value.toIntOrNull() ?: 100; widgets = true }
            "widgetAccent" -> { prefs.widgetAccent = value; widgets = true }
            "widgetPrivacy" -> { prefs.widgetPrivacy = value == "true"; widgets = true }
            "healthOn" -> { prefs.healthOn = value == "true"; HealthSync.schedule(act) }
            "healthTime" -> { prefs.healthTime = value; HealthSync.schedule(act) }
            "configDir" -> { prefs.configDir = ""; restart = true } // back to the app's own storage
        }
        AppConfig.save(act) // keep settings.json in the config folder current
        if (restart) act.runOnUiThread { act.restartEngine() }
        if (widgets) Refresh.redraw(act)
        return snapshot().toString()
    }

    @JavascriptInterface fun pickFolder() = act.runOnUiThread { act.pickFolder() }
    @JavascriptInterface fun pickConfigFolder() = act.runOnUiThread { act.pickConfigFolder() }
    @JavascriptInterface fun fileAccess() = act.runOnUiThread { act.startActivity(Storage.accessIntent(act)) }
    @JavascriptInterface fun demo() = act.runOnUiThread { act.useDemo() }
    @JavascriptInterface fun choosePin() = act.runOnUiThread { act.choosePin(thenMode = prefs.lockMode) }
    @JavascriptInterface fun lockNow() = act.runOnUiThread { Lock.lockNow(); act.showLockIfNeeded() }
    @JavascriptInterface fun restartEngine() = act.runOnUiThread { act.restartEngine() }

    @JavascriptInterface
    fun refreshWidgets() {
        Thread {
            val ok = Refresh.now(act)
            act.emit("toast", JSONObject().put("text", if (ok) "Widgets refreshed" else "Widgets: ${Refresh.lastError}").put("type", if (ok) "ok" else "error"))
            act.emit("settings", snapshot())
        }.start()
    }

    @JavascriptInterface
    fun log(): String = Engine.tail(act, 80).ifBlank { "(the log is empty)" }

    @JavascriptInterface
    fun appInfo() = act.runOnUiThread {
        act.startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:${act.packageName}")))
    }

    // Status/navigation bar icons follow the page theme.
    @JavascriptInterface
    fun setBars(dark: Boolean) = act.runOnUiThread { act.setBars(dark) }
}

// versionName without BuildConfig (kept off to stay minimal).
object BuildConfigVersion {
    fun name(ctx: android.content.Context): String = try {
        ctx.packageManager.getPackageInfo(ctx.packageName, 0).versionName ?: "?"
    } catch (_: Exception) { "?" }
}
