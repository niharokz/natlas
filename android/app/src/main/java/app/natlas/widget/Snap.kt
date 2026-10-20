package app.natlas.widget

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

// The last snapshot from the engine, kept in the app's files (widgets draw from it).
object Snap {
    private fun file(ctx: Context) = File(ctx.filesDir, "snapshot.json")

    fun save(ctx: Context, s: JSONObject) {
        val tmp = File(ctx.filesDir, "snapshot.json.tmp")
        tmp.writeText(s.toString())
        tmp.renameTo(file(ctx))
    }

    fun load(ctx: Context): JSONObject? = try {
        file(ctx).takeIf { it.exists() }?.readText()?.let { JSONObject(it) }
    } catch (_: Exception) { null }

    fun ageMs(ctx: Context): Long = file(ctx).let { if (it.exists()) System.currentTimeMillis() - it.lastModified() else Long.MAX_VALUE }

    // JSONArray → list of objects.
    fun objs(a: JSONArray?): List<JSONObject> = if (a == null) emptyList() else List(a.length()) { a.optJSONObject(it) ?: JSONObject() }

    fun pluginTitle(snap: JSONObject, id: String): String =
        objs(snap.optJSONArray("plugins")).firstOrNull { it.optString("id") == id }?.optString("title") ?: id

    // Lists the List and Quick add widgets can show: "plugin/coll" → title.
    fun lists(snap: JSONObject?, quickOnly: Boolean): List<Pair<String, String>> {
        if (snap == null) return emptyList()
        val out = mutableListOf<Pair<String, String>>()
        for (p in objs(snap.optJSONArray("plugins"))) {
            for (c in objs(p.optJSONArray("collections"))) {
                if (c.optString("kind") != "list") continue
                if (quickOnly && !c.optBoolean("quick")) continue
                if (!quickOnly && p.optString("view") == "document") continue
                val label = if (c.optString("label") == p.optString("title")) p.optString("title") else p.optString("title") + " · " + c.optString("label")
                out += "${p.optString("id")}/${c.optString("id")}" to label
            }
        }
        return out
    }
}
