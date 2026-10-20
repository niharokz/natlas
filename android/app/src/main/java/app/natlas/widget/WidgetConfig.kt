package app.natlas.widget

import android.app.Activity
import android.appwidget.AppWidgetManager
import android.content.Intent
import android.os.Bundle
import android.view.Gravity
import android.widget.LinearLayout
import android.widget.ScrollView
import app.natlas.Prefs
import app.natlas.R
import app.natlas.Refresh
import app.natlas.Ui
import org.json.JSONObject

// "Which list / number?" when a List, Quick add, Number or Glance widget is
// placed (and on long-press → reconfigure). Backing out keeps the default.
class WidgetConfig : Activity() {
    private var id = AppWidgetManager.INVALID_APPWIDGET_ID

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        id = intent.getIntExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, AppWidgetManager.INVALID_APPWIDGET_ID)
        if (id == AppWidgetManager.INVALID_APPWIDGET_ID) { finish(); return }
        setResult(RESULT_OK, Intent().putExtra(AppWidgetManager.EXTRA_APPWIDGET_ID, id)) // the default is fine too
        val snap = Snap.load(this)
        if (snap != null) show(snap) else {
            setContentView(Ui.text(this, "Loading your lists…", 16f, R.color.muted).apply { setPadding(60, 60, 60, 60) })
            Thread {
                Refresh.now(applicationContext)
                runOnUiThread { Snap.load(this)?.let { show(it) } ?: done() }
            }.start()
        }
    }

    private fun kind(): Kind? = Kind.of(AppWidgetManager.getInstance(this).getAppWidgetInfo(id)?.provider?.className)

    private fun options(snap: JSONObject): Pair<String, List<Pair<String, String>>> = when (kind()) {
        Kind.LIST -> "Show which list?" to Snap.lists(snap, quickOnly = false)
        Kind.QUICK -> "Add to which list?" to Snap.lists(snap, quickOnly = true)
        Kind.NUMBER -> "Show which number?" to Snap.objs(snap.optJSONArray("stats")).map {
            "${it.optString("plugin")}|${it.optString("coll")}|${it.optString("label")}" to "${it.optString("title")} · ${it.optString("label")}  (${it.optString("value")})"
        }
        Kind.GLANCE -> "Numbers of which list?" to Snap.objs(snap.optJSONArray("stats")).map { it.optString("plugin") }.distinct()
            .map { it to Snap.pluginTitle(snap, it) }
        else -> "" to emptyList()
    }

    private fun show(snap: JSONObject) {
        val (title, opts) = options(snap)
        if (opts.isEmpty()) { done(); return }
        val col = Ui.column(this)
        val pad = Ui.dp(this, 20f)
        col.setPadding(pad, pad, pad, pad)
        col.gravity = Gravity.START
        col.addView(Ui.text(this, title, 18f, R.color.fg, bold = true).apply { gravity = Gravity.START })
        val current = Prefs(this).widgetSource(id)
        for ((key, label) in opts) {
            val b = Ui.button(this, if (key == current) "✓  $label" else label, primary = key == current) { pick(key) }
            b.gravity = Gravity.START or Gravity.CENTER_VERTICAL
            col.addView(b, Ui.lp(this, 10f, true))
        }
        setContentView(ScrollView(this).apply {
            setBackgroundColor(Ui.color(this@WidgetConfig, R.color.bg))
            addView(col, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT))
        })
    }

    private fun pick(key: String) {
        Prefs(this).setWidgetSource(id, key)
        done()
    }

    private fun done() {
        val k = kind()
        if (k != null) AppWidgetManager.getInstance(this).updateAppWidget(id, Render.build(this, k, id, Snap.load(this)))
        finish()
    }
}
