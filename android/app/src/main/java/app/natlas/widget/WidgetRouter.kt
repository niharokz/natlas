package app.natlas.widget

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import app.natlas.Engine
import app.natlas.MainActivity
import app.natlas.Prefs
import app.natlas.Refresh
import org.json.JSONObject

// Invisible screen behind list rows on widgets: a tick runs the action
// straight away (with Undo on the widget for a few seconds), a tap opens the app.
class WidgetRouter : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val app = applicationContext
        when (intent.getStringExtra("do")) {
            "open" -> startActivity(Intent(this, MainActivity::class.java)
                .putExtra(MainActivity.EXTRA_ROUTE, intent.getStringExtra("route") ?: "#/today")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP))
            "tick" -> intent.getStringExtra("req")?.let { req -> Thread { tick(app, JSONObject(req)) }.start() }
            "undo" -> Thread { undo(app) }.start()
        }
        finish()
        @Suppress("DEPRECATION") overridePendingTransition(0, 0)
    }

    companion object {
        private const val UNDO_MS = 6_000L
        private val main = Handler(Looper.getMainLooper())

        private fun toast(ctx: Context, text: String) = main.post { Toast.makeText(ctx, text, Toast.LENGTH_SHORT).show() }

        fun tick(ctx: Context, req: JSONObject) {
            val res = Engine.cli(ctx, JSONObject(req.toString()).apply { remove("label"); remove("title") })
            val prefs = Prefs(ctx)
            if (res.optInt("status") == 200) {
                val rec = res.optJSONObject("record") ?: JSONObject()
                prefs.undo = JSONObject().put("op", "patch").put("plugin", req.optString("plugin")).put("coll", req.optString("coll"))
                    .put("id", rec.optString("_id", req.optString("id"))).put("rev", rec.optString("_rev"))
                    .put("values", res.optJSONObject("undo") ?: JSONObject())
                    .put("label", req.optString("label", "Done")).put("title", req.optString("title")).toString()
                prefs.undoUntil = System.currentTimeMillis() + UNDO_MS
            } else {
                toast(ctx, res.optString("error", "Could not save").take(140))
            }
            Refresh.now(ctx)
            main.postDelayed({ Refresh.redraw(ctx) }, UNDO_MS + 300) // hide the Undo bar again
        }

        fun undo(ctx: Context) {
            val prefs = Prefs(ctx)
            if (prefs.undo.isEmpty()) return
            val req = JSONObject(prefs.undo)
            prefs.undo = ""
            prefs.undoUntil = 0
            val res = Engine.cli(ctx, JSONObject(req.toString()).apply { remove("label"); remove("title") })
            toast(ctx, if (res.optInt("status") == 200) "Undone" else res.optString("error", "Could not undo").take(140))
            Refresh.now(ctx)
        }
    }
}
