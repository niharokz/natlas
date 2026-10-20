package app.natlas.widget

import android.appwidget.AppWidgetManager
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import app.natlas.Prefs
import app.natlas.Refresh

// Widget buttons that need no screen (refresh, month arrows), midnight and boot.
class WidgetReceiver : BroadcastReceiver() {
    companion object {
        const val REFRESH = "app.natlas.REFRESH"
        const val MONTH = "app.natlas.MONTH"
        const val MIDNIGHT = "app.natlas.MIDNIGHT"
    }

    override fun onReceive(ctx: Context, intent: Intent) {
        when (intent.action) {
            MONTH -> {
                val id = intent.getIntExtra("id", 0)
                val delta = intent.getIntExtra("delta", 0)
                val prefs = Prefs(ctx)
                prefs.setMonthOffset(id, if (delta == 0) 0 else prefs.monthOffset(id) + delta) // tap the title: back to today
                val mgr = AppWidgetManager.getInstance(ctx)
                if (id in mgr.getAppWidgetIds(ComponentName(ctx, MonthWidget::class.java)))
                    mgr.updateAppWidget(id, Render.build(ctx, Kind.MONTH, id, Snap.load(ctx)))
            }
            REFRESH, MIDNIGHT, Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_MY_PACKAGE_REPLACED,
            Intent.ACTION_TIMEZONE_CHANGED, Intent.ACTION_DATE_CHANGED -> {
                if (intent.action != REFRESH) Refresh.scheduleMidnight(ctx)
                val pending = goAsync()
                Thread { try { Refresh.now(ctx.applicationContext) } finally { pending.finish() } }.start()
            }
        }
    }
}
