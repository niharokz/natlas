package app.natlas

import android.app.AlarmManager
import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import app.natlas.widget.Kind
import app.natlas.widget.Render
import app.natlas.widget.Snap
import app.natlas.widget.WidgetReceiver
import org.json.JSONObject
import java.util.Calendar

// Widget data: one snapshot from the engine (`natlas cli`), then every widget
// is redrawn from it. Runs when you leave the app, after a widget tick, every
// 30 min (Android's widget update) and just after midnight.
object Refresh {
    @Volatile var lastError = ""
        private set

    // Reads a fresh snapshot and redraws every widget. Call off the UI thread.
    fun now(ctx: Context): Boolean {
        val prefs = Prefs(ctx)
        if (prefs.folder.isEmpty()) { lastError = "choose your folder in the app first"; redraw(ctx); return false }
        val res = Engine.cli(ctx, JSONObject().put("op", "snapshot"))
        val ok = res.optInt("status") == 200
        if (ok) {
            Snap.save(ctx, res)
            prefs.lastRefresh = System.currentTimeMillis()
            lastError = ""
        } else {
            lastError = res.optString("error", "unknown error")
        }
        redraw(ctx)
        return ok
    }

    // Redraws every widget from the last snapshot (fast, no engine).
    fun redraw(ctx: Context) {
        val mgr = AppWidgetManager.getInstance(ctx)
        val snap = Snap.load(ctx)
        for (kind in Kind.entries) {
            for (id in mgr.getAppWidgetIds(ComponentName(ctx, kind.provider))) {
                try { mgr.updateAppWidget(id, Render.build(ctx, kind, id, snap)) } catch (_: Exception) { }
            }
        }
    }

    // Once a day just after midnight, so "Today" moves on without opening the app.
    fun scheduleMidnight(ctx: Context) {
        val am = ctx.getSystemService(AlarmManager::class.java) ?: return
        val at = Calendar.getInstance().apply {
            add(Calendar.DAY_OF_YEAR, 1); set(Calendar.HOUR_OF_DAY, 0); set(Calendar.MINUTE, 1); set(Calendar.SECOND, 0)
        }.timeInMillis
        val pi = PendingIntent.getBroadcast(ctx, 7, Intent(ctx, WidgetReceiver::class.java).setAction(WidgetReceiver.MIDNIGHT),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        am.setWindow(AlarmManager.RTC, at, 15 * 60 * 1000L, pi) // inexact: no special permission needed
    }
}
