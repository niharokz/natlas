package app.natlas

import android.content.Context
import android.content.SharedPreferences

// Phone-only settings. They live in the app (not in the synced folder), so the
// phone and the server never fight over them.
class Prefs(ctx: Context) {
    private val p: SharedPreferences = ctx.applicationContext.getSharedPreferences("natlas", Context.MODE_PRIVATE)

    companion object {
        // Built-in lists, in the default menu order.
        val ALL = listOf("events", "subscriptions", "calendar", "health", "inventory", "architecture")
        val TITLES = mapOf(
            "events" to "Events", "subscriptions" to "Subscriptions", "calendar" to "Calendar",
            "health" to "Health", "inventory" to "Inventory", "architecture" to "Architecture",
        )
        val DELAYS = listOf(0, 60, 300, 900, -1)
    }

    // raw values (AppConfig copies some of them to settings.json)
    fun raw(): Map<String, *> = p.all
    fun editor(): SharedPreferences.Editor = p.edit()

    // Settings → Config folder: natlas.yml, plugins/, settings.json ("" = inside the app)
    var configDir: String
        get() = p.getString("configDir", "") ?: ""
        set(v) = p.edit().putString("configDir", v).apply()

    var folder: String
        get() = p.getString("folder", "") ?: ""
        set(v) = p.edit().putString("folder", v).apply()

    // All list ids in menu order (enabled or not).
    var order: List<String>
        get() {
            val saved = (p.getString("order", "") ?: "").split(",").filter { it in ALL }
            return saved + ALL.filter { it !in saved }
        }
        set(v) = p.edit().putString("order", v.filter { it in ALL }.joinToString(",")).apply()

    var enabled: Set<String>
        get() = (p.getString("enabled", null) ?: ALL.joinToString(",")).split(",").filter { it in ALL }.toSet()
        set(v) = p.edit().putString("enabled", v.joinToString(",")).apply()

    // NATLAS_PLUGINS for the engine: enabled lists in menu order.
    val plugins: String get() = order.filter { it in enabled }.joinToString(",")

    var start: String
        get() = p.getString("start", "#/today") ?: "#/today"
        set(v) = p.edit().putString("start", if (v.startsWith("#/")) v else "#/today").apply()

    // off | device (fingerprint or screen lock) | pin
    var lockMode: String
        get() = p.getString("lock", "off") ?: "off"
        set(v) = p.edit().putString("lock", if (v in listOf("off", "device", "pin")) v else "off").apply()

    var pinHash: String
        get() = p.getString("pinHash", "") ?: ""
        set(v) = p.edit().putString("pinHash", v).apply()

    var pinLength: Int
        get() = p.getInt("pinLength", 0)
        set(v) = p.edit().putInt("pinLength", v).apply()

    var pinBio: Boolean
        get() = p.getBoolean("pinBio", true)
        set(v) = p.edit().putBoolean("pinBio", v).apply()

    // seconds in the background before locking again; -1 = only after a restart
    var lockDelay: Int
        get() = p.getInt("delay", 60)
        set(v) = p.edit().putInt("delay", if (v in DELAYS) v else 60).apply()

    var secure: Boolean
        get() = p.getBoolean("secure", true)
        set(v) = p.edit().putBoolean("secure", v).apply()

    var animations: Boolean
        get() = p.getBoolean("animations", true)
        set(v) = p.edit().putBoolean("animations", v).apply()

    var main: Boolean
        get() = p.getBoolean("main", false)
        set(v) = p.edit().putBoolean("main", v).apply()

    var port: Int
        get() = p.getInt("port", 0)
        set(v) = p.edit().putInt("port", v).apply()

    var pinFails: Int
        get() = p.getInt("pinFails", 0)
        set(v) = p.edit().putInt("pinFails", v).apply()

    var pinPausedUntil: Long
        get() = p.getLong("pinPaused", 0)
        set(v) = p.edit().putLong("pinPaused", v).apply()

    // widgets: auto | light | dark | amoled | glass | clear
    var widgetTheme: String
        get() = p.getString("wTheme", "auto") ?: "auto"
        set(v) = p.edit().putString("wTheme", v).apply()

    var widgetOpacity: Int
        get() = p.getInt("wOpacity", 100)
        set(v) = p.edit().putInt("wOpacity", v.coerceIn(0, 100)).apply()

    // teal | dynamic (Material You)
    var widgetAccent: String
        get() = p.getString("wAccent", "teal") ?: "teal"
        set(v) = p.edit().putString("wAccent", if (v == "dynamic") "dynamic" else "teal").apply()

    var widgetPrivacy: Boolean
        get() = p.getBoolean("wPrivacy", false)
        set(v) = p.edit().putBoolean("wPrivacy", v).apply()

    var lastRefresh: Long
        get() = p.getLong("wLast", 0)
        set(v) = p.edit().putLong("wLast", v).apply()

    // per widget: which list / number it shows ("plugin/coll", "plugin|label", …)
    fun widgetSource(id: Int): String = p.getString("w.$id.src", "") ?: ""
    fun setWidgetSource(id: Int, v: String) = p.edit().putString("w.$id.src", v).apply()

    // month widget: months away from today
    fun monthOffset(id: Int): Int = p.getInt("w.$id.month", 0)
    fun setMonthOffset(id: Int, v: Int) = p.edit().putInt("w.$id.month", v).apply()

    fun forgetWidget(id: Int) = p.edit().remove("w.$id.src").remove("w.$id.month").apply()

    // Undo bar after a tick on a widget: JSON request + when it disappears
    var undo: String
        get() = p.getString("undo", "") ?: ""
        set(v) = p.edit().putString("undo", v).apply()

    var undoUntil: Long
        get() = p.getLong("undoUntil", 0)
        set(v) = p.edit().putLong("undoUntil", v).apply()

    // Health Connect sync (Settings → Health)
    var healthOn: Boolean
        get() = p.getBoolean("hOn", false)
        set(v) = p.edit().putBoolean("hOn", v).apply()

    // HH:MM, local time
    var healthTime: String
        get() = p.getString("hTime", "07:00") ?: "07:00"
        set(v) = p.edit().putString("hTime", if (Regex("^([01]\\d|2[0-3]):[0-5]\\d$").matches(v)) v else "07:00").apply()

    var healthNote: String
        get() = p.getString("hNote", "") ?: ""
        set(v) = p.edit().putString("hNote", v).apply()

    var healthLast: Long
        get() = p.getLong("hLast", 0)
        set(v) = p.edit().putLong("hLast", v).apply()

    // ISO day of the last successful morning sync
    var healthDay: String
        get() = p.getString("hDay", "") ?: ""
        set(v) = p.edit().putString("hDay", v).apply()

    // steps per source on the last synced day ("Samsung Health · phone 9,812 · … → counted 19,120")
    var healthSources: String
        get() = p.getString("hSources", "") ?: ""
        set(v) = p.edit().putString("hSources", v).apply()

    var healthImported: Boolean
        get() = p.getBoolean("hImported", false)
        set(v) = p.edit().putBoolean("hImported", v).apply()
}
