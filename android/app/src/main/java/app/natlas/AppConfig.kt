package app.natlas

import android.content.Context
import org.json.JSONObject
import java.io.File

// The app's config folder (Settings → Config folder): natlas.yml and plugins/
// for the engine (both optional), and settings.json, a copy of these settings
// written on every change. Pick the same folder after a reinstall (or on a new
// phone) and the settings come back. PIN, widget choices and health sync state
// stay in the app: they belong to this install.
object AppConfig {
    private const val FILE = "settings.json"

    // Settings kept in settings.json, by their Prefs key.
    private val STRINGS = listOf("folder", "order", "enabled", "start", "lock", "wTheme", "wAccent", "hTime")
    private val INTS = listOf("delay", "wOpacity")
    private val BOOLS = listOf("pinBio", "secure", "animations", "main", "wPrivacy", "hOn")

    fun dir(ctx: Context): File? = Prefs(ctx).configDir.takeIf { it.isNotEmpty() }?.let { File(it) }

    // engine config file and plugin folder: the config folder, else the app's own
    fun configFile(ctx: Context) = File(dir(ctx) ?: ctx.filesDir, "natlas.yml")
    fun pluginsDir(ctx: Context) = File(dir(ctx) ?: ctx.filesDir, "plugins")

    // Writes settings.json (atomically). Quietly does nothing without a config folder.
    fun save(ctx: Context): Boolean {
        val d = dir(ctx) ?: return false
        return try {
            val raw = Prefs(ctx).raw()
            val o = JSONObject().put("app", "natlas").put("version", 1)
            for (k in STRINGS + INTS + BOOLS) raw[k]?.let { o.put(k, it) }
            d.mkdirs()
            val tmp = File(d, ".$FILE.tmp")
            tmp.writeText(o.toString(2))
            tmp.renameTo(File(d, FILE))
        } catch (_: Exception) { false }
    }

    // Reads settings.json from the config folder into the app; false when there is none.
    fun load(ctx: Context): Boolean {
        val f = File(dir(ctx) ?: return false, FILE)
        if (!f.isFile) return false
        return try {
            val o = JSONObject(f.readText())
            val p = Prefs(ctx)
            val e = p.editor()
            for (k in STRINGS) if (o.has(k)) e.putString(k, o.getString(k))
            for (k in INTS) if (o.has(k)) e.putInt(k, o.getInt(k))
            for (k in BOOLS) if (o.has(k)) e.putBoolean(k, o.getBoolean(k))
            e.apply()
            if (p.lockMode == "pin" && p.pinHash.isEmpty()) p.lockMode = "device" // the PIN never leaves the phone
            true
        } catch (_: Exception) { false }
    }

    // One line for Settings.
    fun describe(ctx: Context): String {
        val d = dir(ctx) ?: return "In the app (lost on uninstall). Choose a folder to keep a copy."
        if (!d.isDirectory) return "Folder missing: ${d.path}"
        val has = listOfNotNull(FILE.takeIf { File(d, it).isFile }, "natlas.yml".takeIf { File(d, it).isFile },
            "plugins/".takeIf { File(d, "plugins").isDirectory })
        return if (has.isEmpty()) "Empty: settings.json is written on the next change" else "Has " + has.joinToString(", ")
    }
}
