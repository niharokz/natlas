package app.natlas.widget

import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.Context
import android.os.Bundle
import app.natlas.Prefs
import app.natlas.Refresh

// Every widget kind. needsSource: it asks which list / number to show.
enum class Kind(val provider: Class<out BaseWidget>, val needsSource: Boolean) {
    TODAY(TodayWidget::class.java, false),
    LIST(ListWidget::class.java, true),
    AGENDA(AgendaWidget::class.java, false),
    MONTH(MonthWidget::class.java, false),
    UPNEXT(UpNextWidget::class.java, false),
    QUICK(QuickWidget::class.java, true),
    NUMBER(NumberWidget::class.java, true),
    GLANCE(GlanceWidget::class.java, true),
    CLOCK(ClockWidget::class.java, false),
    STEPS(StepsWidget::class.java, false),
    SLEEP(SleepWidget::class.java, false),
    WEEK(WeekWidget::class.java, false),
    WEIGHT(WeightWidget::class.java, false);

    companion object {
        fun of(provider: String?): Kind? = entries.firstOrNull { it.provider.name == provider }
    }
}

// Draws from the last snapshot at once, then fetches a fresh one if it is old.
abstract class BaseWidget : AppWidgetProvider() {
    abstract val kind: Kind

    override fun onUpdate(ctx: Context, mgr: AppWidgetManager, ids: IntArray) {
        val snap = Snap.load(ctx)
        for (id in ids) mgr.updateAppWidget(id, Render.build(ctx, kind, id, snap))
        if (Snap.ageMs(ctx) > 10 * 60 * 1000L) {
            val pending = goAsync()
            Thread { try { Refresh.now(ctx.applicationContext) } finally { pending.finish() } }.start()
        }
    }

    override fun onAppWidgetOptionsChanged(ctx: Context, mgr: AppWidgetManager, id: Int, newOptions: Bundle) {
        mgr.updateAppWidget(id, Render.build(ctx, kind, id, Snap.load(ctx)))
    }

    override fun onDeleted(ctx: Context, ids: IntArray) {
        val prefs = Prefs(ctx)
        for (id in ids) prefs.forgetWidget(id)
    }
}

class TodayWidget : BaseWidget() { override val kind get() = Kind.TODAY }
class ListWidget : BaseWidget() { override val kind get() = Kind.LIST }
class AgendaWidget : BaseWidget() { override val kind get() = Kind.AGENDA }
class MonthWidget : BaseWidget() { override val kind get() = Kind.MONTH }
class UpNextWidget : BaseWidget() { override val kind get() = Kind.UPNEXT }
class QuickWidget : BaseWidget() { override val kind get() = Kind.QUICK }
class NumberWidget : BaseWidget() { override val kind get() = Kind.NUMBER }
class GlanceWidget : BaseWidget() { override val kind get() = Kind.GLANCE }
class ClockWidget : BaseWidget() { override val kind get() = Kind.CLOCK }
class StepsWidget : BaseWidget() { override val kind get() = Kind.STEPS }
class SleepWidget : BaseWidget() { override val kind get() = Kind.SLEEP }
class WeekWidget : BaseWidget() { override val kind get() = Kind.WEEK }
class WeightWidget : BaseWidget() { override val kind get() = Kind.WEIGHT }
