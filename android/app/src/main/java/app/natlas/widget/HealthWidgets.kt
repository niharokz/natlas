package app.natlas.widget

import android.appwidget.AppWidgetManager
import android.content.Context
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.view.View
import android.widget.RemoteViews
import app.natlas.R
import org.json.JSONObject
import java.time.LocalDate
import java.time.LocalTime
import java.time.format.DateTimeFormatter
import java.time.format.TextStyle
import java.time.temporal.ChronoUnit
import java.util.Locale
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.floor
import kotlin.math.log10
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.roundToInt

// The four health widgets (Steps ring, Sleep, Steps week, Weight), drawn from the
// snapshot's `charts` entry (Health → Daily). Charts are alpha masks tinted with the
// widget colour pairs, so they follow light/dark like the text, and are sized to the
// widget so they stay small enough for Android to pass to the launcher.
object HealthWidgets {
    // Stage colours: the same validated palette as the app (light slots read on both).
    private val STAGES = intArrayOf(Color.parseColor("#2A78D6"), Color.parseColor("#EB6834"), Color.parseColor("#1BAF7A"), Color.parseColor("#EDA100"))

    private class Data(val c: JSONObject) {
        val days: List<JSONObject> = Snap.objs(c.optJSONArray("days"))
        val goals: JSONObject = c.optJSONObject("goals") ?: JSONObject()
        val route = "#/p/${c.optString("plugin")}/${c.optString("coll")}"
        fun num(d: JSONObject, k: String): Double? = if (d.has(k) && !d.isNull(k)) d.optDouble(k).takeIf { !it.isNaN() } else null
        fun latest(k: String): JSONObject? = days.lastOrNull { num(it, k) != null }
        fun goal(k: String): Double? = goals.optDouble(k).takeIf { !it.isNaN() && it > 0 }
    }

    private fun data(snap: JSONObject): Data? {
        val list = Snap.objs(snap.optJSONArray("charts"))
        return (list.firstOrNull { it.optBoolean("sync") } ?: list.firstOrNull())?.let { Data(it) }
    }

    // Widget size in dp (portrait width x height), with a default before the launcher says.
    private fun size(ctx: Context, id: Int, w: Int, h: Int): Pair<Int, Int> {
        val o = AppWidgetManager.getInstance(ctx).getAppWidgetOptions(id)
        val ow = o.getInt(AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH, 0)
        val oh = o.getInt(AppWidgetManager.OPTION_APPWIDGET_MAX_HEIGHT, 0)
        return (if (ow > 0) ow else w) to (if (oh > 0) oh else h)
    }

    // Pixels per dp for mask bitmaps: sharp enough, never huge.
    private fun scale(ctx: Context): Float = min(ctx.resources.displayMetrics.density, 2f).also { dpi = (it * 160).roundToInt() }

    private fun hours(h: Double): String { val m = (h * 60).roundToInt(); return "${m / 60}h ${"%02d".format(m % 60)}m" }
    private fun fmt(v: Double, dec: Int = 0): String = if (dec == 0) "%,d".format(v.roundToInt()) else "%,.${dec}f".format(v)
    private fun when_(snap: JSONObject, iso: String): String {
        val n = try { ChronoUnit.DAYS.between(LocalDate.parse(snap.optString("today")), LocalDate.parse(iso)) } catch (_: Exception) { 99 }
        return when (n) {
            0L -> "Today"; -1L -> "Yesterday"
            else -> try { LocalDate.parse(iso).format(DateTimeFormatter.ofPattern("EEE d MMM", Locale.getDefault())) } catch (_: Exception) { iso }
        }
    }
    private fun clock(t: String): String = try {
        LocalTime.parse(t).format(DateTimeFormatter.ofPattern("h:mm a", Locale.getDefault())).lowercase()
    } catch (_: Exception) { t }

    private fun empty(ctx: Context, look: Look, id: Int, text: String, route: String, open: (String, Int) -> android.app.PendingIntent): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_message)
        look.background(rv, R.id.bg)
        rv.setTextViewText(R.id.text, text)
        look.text(rv, R.id.text, look.muted)
        rv.setOnClickPendingIntent(R.id.root, open(route, id * 1000 + 950))
        return rv
    }

    // Bitmaps say how many pixels they hold per dp, so a wrap_content ImageView shows them at the size drawn.
    private var dpi = 320
    private fun mask(wpx: Int, hpx: Int) = Bitmap.createBitmap(max(wpx, 1), max(hpx, 1), Bitmap.Config.ALPHA_8).apply { density = dpi }
    private fun ink() = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.BLACK }

    // ── Steps ring (1×1 … 4×2)

    fun steps(ctx: Context, look: Look, id: Int, snap: JSONObject, open: (String, Int) -> android.app.PendingIntent): RemoteViews {
        val d = data(snap) ?: return empty(ctx, look, id, "Turn on Health in Natlas", "#/phone", open)
        val day = d.latest("steps") ?: return empty(ctx, look, id, "No steps yet. Natlas → Settings → Health", d.route, open)
        val v = d.num(day, "steps") ?: 0.0
        val goal = d.goal("steps")
        val pct = if (goal != null) v / goal else 0.0
        val (wdp, hdp) = size(ctx, id, 110, 110)
        val rv = RemoteViews(ctx.packageName, R.layout.w_health_ring)
        look.background(rv, R.id.bg)
        val ringDp = max(40, min(min(wdp, hdp) - 24, 150))
        val s = scale(ctx)
        val px = (ringDp * s).roundToInt()
        val stroke = max(5f, ringDp * 0.09f) * s
        val track = mask(px, px); val fill = mask(px, px)
        val oval = RectF(stroke / 2, stroke / 2, px - stroke / 2, px - stroke / 2)
        val p = ink().apply { style = Paint.Style.STROKE; strokeWidth = stroke; strokeCap = Paint.Cap.ROUND }
        Canvas(track).drawArc(oval, 0f, 360f, false, p)
        if (pct > 0) Canvas(fill).drawArc(oval, -90f, (360 * min(pct, 1.0)).toFloat(), false, p)
        rv.setImageViewBitmap(R.id.ring_track, track); rv.setImageViewBitmap(R.id.ring_fill, fill)
        rv.setColorInt(R.id.ring_track, "setColorFilter", look.chip.first, look.chip.second)
        val fc = if (pct >= 1) look.tone("ok") else look.accent
        rv.setColorInt(R.id.ring_fill, "setColorFilter", fc.first, fc.second)
        val small = ringDp < 80
        rv.setTextViewText(R.id.ring_value, if (v >= 10000 && small) "%.1fK".format(v / 1000) else fmt(v))
        rv.setTextViewTextSize(R.id.ring_value, android.util.TypedValue.COMPLEX_UNIT_SP, (ringDp * 0.2f).coerceIn(11f, 26f))
        rv.setTextViewText(R.id.ring_label, "steps")
        rv.setViewVisibility(R.id.ring_label, if (small) View.GONE else View.VISIBLE)
        look.text(rv, R.id.ring_value, look.text); look.text(rv, R.id.ring_label, look.muted)
        // details beside the ring when the widget is wide, under it when tall
        val wide = wdp >= ringDp + 120
        rv.setViewVisibility(R.id.side, if (wide) View.VISIBLE else View.GONE)
        val goalText = if (goal != null) (if (pct >= 1) "✓ Goal ${fmt(goal)}" else "${(pct * 100).roundToInt()}% of ${fmt(goal)}") else ""
        val extra = listOfNotNull(d.num(day, "km")?.let { fmt(it, 1) + " km" }, d.num(day, "kcal_active")?.let { fmt(it) + " kcal" }).joinToString(" · ")
        rv.setTextViewText(R.id.side_title, "Steps · " + when_(snap, day.optString("date")))
        rv.setTextViewText(R.id.side_goal, goalText)
        rv.setTextViewText(R.id.side_extra, extra)
        look.text(rv, R.id.side_title, look.muted); look.text(rv, R.id.side_goal, if (pct >= 1) look.tone("ok") else look.text); look.text(rv, R.id.side_extra, look.muted)
        val foot = !wide && hdp >= ringDp + 34
        rv.setViewVisibility(R.id.foot, if (foot) View.VISIBLE else View.GONE)
        rv.setTextViewText(R.id.foot, if (goal != null) "${(pct * 100).roundToInt()}% · ${when_(snap, day.optString("date"))}" else when_(snap, day.optString("date")))
        look.text(rv, R.id.foot, look.muted)
        rv.setOnClickPendingIntent(R.id.root, open(d.route, id * 1000 + 951))
        return rv
    }

    // ── Sleep (2×2, 4×2): last night + the week's nights when wide

    fun sleep(ctx: Context, look: Look, id: Int, snap: JSONObject, open: (String, Int) -> android.app.PendingIntent): RemoteViews {
        val d = data(snap) ?: return empty(ctx, look, id, "Turn on Health in Natlas", "#/phone", open)
        val hk = d.c.optJSONObject("sleep")?.optString("hours") ?: "sleep_h"
        val night = d.latest(hk) ?: return empty(ctx, look, id, "No sleep yet. Natlas → Settings → Health", d.route, open)
        val (wdp, hdp) = size(ctx, id, 250, 110)
        val rv = RemoteViews(ctx.packageName, R.layout.w_health_sleep)
        look.background(rv, R.id.bg)
        val h = d.num(night, hk) ?: 0.0
        rv.setTextViewText(R.id.title, "Sleep · " + when_(snap, night.optString("date")).let { if (it == "Today") "last night" else it })
        rv.setTextViewText(R.id.value, hours(h))
        val goal = d.goal(hk)
        val bed = night.optString("sleep_start"); val wake = night.optString("sleep_end")
        rv.setTextViewText(R.id.times, if (bed.isNotEmpty() && wake.isNotEmpty()) "${clock(bed)} – ${clock(wake)}" else "")
        look.text(rv, R.id.title, look.muted); look.text(rv, R.id.value, if (goal != null && h >= goal) look.tone("ok") else look.text); look.text(rv, R.id.times, look.muted)
        // stage bar in its real colours
        val keys = listOf("deep_h" to "Deep", "rem_h" to "REM", "light_h" to "Light", "awake_h" to "Awake")
        val parts = keys.mapIndexedNotNull { i, (k, label) -> d.num(night, k)?.takeIf { it > 0 }?.let { Triple(i, label, it) } }
        val s = scale(ctx)
        val leftDp = if (wdp >= 230) wdp / 2 - 20 else wdp - 32
        if (parts.isNotEmpty()) {
            val bw = (leftDp * s).roundToInt(); val bh = (8 * s).roundToInt()
            val bar = Bitmap.createBitmap(max(bw, 1), max(bh, 1), Bitmap.Config.ARGB_8888).apply { density = dpi }
            val cv = Canvas(bar); val sum = parts.sumOf { it.third }; val gap = 2 * s
            var x = 0f
            val p = Paint(Paint.ANTI_ALIAS_FLAG)
            parts.forEachIndexed { n, (i, _, v) ->
                val w = ((v / sum) * (bw - gap * (parts.size - 1))).toFloat()
                p.color = STAGES[i]
                cv.drawRoundRect(RectF(x, 0f, x + w, bh.toFloat()), bh / 2f, bh / 2f, p)
                x += w + if (n < parts.size - 1) gap else 0f
            }
            rv.setImageViewBitmap(R.id.stages, bar)
            rv.setTextViewText(R.id.legend, parts.joinToString("  ") { (_, label, v) -> "$label ${hours(v).replace(" ", "")}" })
            look.text(rv, R.id.legend, look.muted)
        } else {
            rv.setViewVisibility(R.id.stages, View.GONE); rv.setViewVisibility(R.id.legend, View.GONE)
        }
        // the last 7 nights as bars, with the goal
        val wide = wdp >= 230 && hdp >= 90
        rv.setViewVisibility(R.id.chart_holder, if (wide) View.VISIBLE else View.GONE)
        if (wide) {
            val end = LocalDate.parse(night.optString("date"))
            val week = (6 downTo 0).map { end.minusDays(it.toLong()) }
            val vals = week.map { day -> d.days.firstOrNull { it.optString("date") == day.toString() }?.let { d.num(it, hk) } }
            val labels = week.map { it.dayOfWeek.getDisplayName(TextStyle.NARROW, Locale.getDefault()) }
            chart(ctx, rv, look, wdp / 2 - 16, hdp - 30, vals, labels, goal, bars = true) { v -> "${v.roundToInt()}h" }
        }
        rv.setOnClickPendingIntent(R.id.root, open(d.route, id * 1000 + 952))
        return rv
    }

    // ── Steps, last 7 days (3×2 … 5×3)

    fun week(ctx: Context, look: Look, id: Int, snap: JSONObject, open: (String, Int) -> android.app.PendingIntent): RemoteViews {
        val d = data(snap) ?: return empty(ctx, look, id, "Turn on Health in Natlas", "#/phone", open)
        val last = d.latest("steps") ?: return empty(ctx, look, id, "No steps yet. Natlas → Settings → Health", d.route, open)
        val (wdp, hdp) = size(ctx, id, 250, 110)
        val rv = RemoteViews(ctx.packageName, R.layout.w_health_chart)
        look.background(rv, R.id.bg)
        val end = LocalDate.parse(last.optString("date"))
        val week = (6 downTo 0).map { end.minusDays(it.toLong()) }
        val vals = week.map { day -> d.days.firstOrNull { it.optString("date") == day.toString() }?.let { d.num(it, "steps") } }
        val got = vals.filterNotNull()
        val avg = if (got.isEmpty()) 0.0 else got.average()
        val goal = d.goal("steps")
        rv.setTextViewText(R.id.title, "Steps · last 7 days")
        rv.setTextViewText(R.id.value, fmt(avg))
        rv.setTextViewText(R.id.sub, "a day" + (goal?.let { g -> " · goal met ${got.count { it >= g }} of ${got.size}" } ?: ""))
        look.text(rv, R.id.title, look.muted); look.text(rv, R.id.value, look.text); look.text(rv, R.id.sub, look.muted)
        val labels = week.map { it.dayOfWeek.getDisplayName(TextStyle.SHORT, Locale.getDefault()).take(2) }
        chart(ctx, rv, look, wdp - 32, hdp - 58, vals, labels, goal, bars = true) { v -> if (v >= 1000) "${(v / 1000).roundToInt()}K" else v.roundToInt().toString() }
        rv.setOnClickPendingIntent(R.id.root, open(d.route, id * 1000 + 953))
        return rv
    }

    // ── Weight trend, last 30 days (2×2 … 4×2)

    fun weight(ctx: Context, look: Look, id: Int, snap: JSONObject, open: (String, Int) -> android.app.PendingIntent): RemoteViews {
        val d = data(snap) ?: return empty(ctx, look, id, "Turn on Health in Natlas", "#/phone", open)
        val latest = d.c.optJSONObject("latest")?.optJSONObject("weight")
            ?: return empty(ctx, look, id, "No weight yet. Weigh in with Samsung Health", d.route, open)
        val (wdp, hdp) = size(ctx, id, 250, 110)
        val rv = RemoteViews(ctx.packageName, R.layout.w_health_chart)
        look.background(rv, R.id.bg)
        val v = latest.optDouble("value")
        val end = LocalDate.parse(snap.optString("today").ifEmpty { latest.optString("date") })
        val from = end.minusDays(29)
        val pts = d.days.mapNotNull { day ->
            val dd = LocalDate.parse(day.optString("date"))
            d.num(day, "weight")?.takeIf { !dd.isBefore(from) }?.let { ChronoUnit.DAYS.between(from, dd).toFloat() / 29f to it }
        }
        rv.setTextViewText(R.id.title, "Weight · " + when_(snap, latest.optString("date")))
        rv.setTextViewText(R.id.value, look.mask(fmt(v, 1) + " kg"))
        val change = if (pts.size > 1) pts.last().second - pts.first().second else null
        rv.setTextViewText(R.id.sub, change?.let { c -> (if (c > 0) "+" else if (c < 0) "−" else "±") + fmt(abs(c), 1) + " kg in 30 days" } ?: "")
        look.text(rv, R.id.title, look.muted); look.text(rv, R.id.value, look.text)
        look.text(rv, R.id.sub, if (change == null || abs(change) < 0.05) look.muted else if (change < 0) look.tone("ok") else look.tone("bad"))
        val tall = hdp >= 100
        rv.setViewVisibility(R.id.chart_box, if (tall && pts.size > 1 && !look.privacy) View.VISIBLE else View.GONE)
        if (tall && pts.size > 1 && !look.privacy) line(ctx, rv, look, wdp - 32, hdp - 58, pts)
        rv.setOnClickPendingIntent(R.id.root, open(d.route, id * 1000 + 954))
        return rv
    }

    // ── drawing: three mask layers (grid+labels, marks, goal) tinted per theme

    private fun nice(hi: Double): Pair<Double, Double> { // top, step
        val raw = max(hi, 1.0) / 3
        val pow = 10.0.pow(floor(log10(raw)))
        val step = listOf(1.0, 2.0, 2.5, 5.0, 10.0).map { it * pow }.first { it >= raw }
        return ceil(max(hi, 1.0) / step) * step to step
    }

    private fun chart(ctx: Context, rv: RemoteViews, look: Look, wdp: Int, hdp: Int, vals: List<Double?>, labels: List<String>,
                      goal: Double?, bars: Boolean, tick: (Double) -> String) {
        val s = scale(ctx)
        val w = (max(wdp, 60) * s).roundToInt(); val h = (max(hdp, 40) * s).roundToInt()
        val grid = mask(w, h); val marks = mask(w, h); val good = mask(w, h)
        val gc = Canvas(grid); val mc = Canvas(marks); val oc = Canvas(good)
        val text = ink().apply { textSize = 9 * s; textAlign = Paint.Align.CENTER }
        val left = 0f; val bottom = h - 12 * s; val top = 4 * s
        val (hi, step) = nice(max(vals.filterNotNull().maxOrNull() ?: 1.0, goal ?: 0.0))
        val y = { v: Double -> (bottom - (v / hi) * (bottom - top)).toFloat() }
        val line = ink().apply { strokeWidth = max(1f, s * 0.6f); alpha = 90 }
        var t = 0.0
        while (t <= hi + step / 2) { gc.drawLine(left, y(t), w.toFloat(), y(t), line); t += step }
        val n = vals.size
        val slot = (w - left) / n
        val barW = min(slot * 0.62f, 22 * s)
        vals.forEachIndexed { i, v ->
            val cx = left + slot * (i + 0.5f)
            gc.drawText(labels[i], cx, h - 2 * s, text)
            if (v == null || v <= 0) return@forEachIndexed
            val r = RectF(cx - barW / 2, y(v), cx + barW / 2, bottom)
            val path = Path().apply { addRoundRect(r, floatArrayOf(4 * s, 4 * s, 4 * s, 4 * s, 0f, 0f, 0f, 0f), Path.Direction.CW) }
            if (bars) (if (goal != null && v >= goal) oc else mc).drawPath(path, ink())
        }
        if (goal != null) {
            val g = ink().apply { strokeWidth = 1.5f * s }
            oc.drawLine(left, y(goal), w.toFloat(), y(goal), g)
        }
        val topLabel = ink().apply { textSize = 8.5f * s; textAlign = Paint.Align.RIGHT }
        gc.drawText(tick(hi), w.toFloat() - 1, top + 8 * s, topLabel)
        layers(rv, look, grid, marks, good)
    }

    private fun line(ctx: Context, rv: RemoteViews, look: Look, wdp: Int, hdp: Int, pts: List<Pair<Float, Double>>) {
        val s = scale(ctx)
        val w = (max(wdp, 60) * s).roundToInt(); val h = (max(hdp, 30) * s).roundToInt()
        val grid = mask(w, h); val marks = mask(w, h); val good = mask(w, h)
        val lo0 = pts.minOf { it.second }; val hi0 = pts.maxOf { it.second }
        val pad = max((hi0 - lo0) * 0.15, 0.3)
        val lo = lo0 - pad; val hi = hi0 + pad
        val top = 4 * s; val bottom = h - 4 * s; val r = 4 * s
        val x = { f: Float -> r + f * (w - 2 * r) }
        val y = { v: Double -> (bottom - (v - lo) / (hi - lo) * (bottom - top)).toFloat() }
        val gl = ink().apply { strokeWidth = max(1f, s * 0.6f); alpha = 90 }
        for (k in 0..2) { val yy = top + (bottom - top) * k / 2; Canvas(grid).drawLine(0f, yy, w.toFloat(), yy, gl) }
        val path = Path()
        pts.forEachIndexed { i, (f, v) -> if (i == 0) path.moveTo(x(f), y(v)) else path.lineTo(x(f), y(v)) }
        Canvas(marks).drawPath(path, ink().apply { style = Paint.Style.STROKE; strokeWidth = 2 * s; strokeJoin = Paint.Join.ROUND; strokeCap = Paint.Cap.ROUND })
        val (lf, lv) = pts.last()
        Canvas(marks).drawCircle(x(lf), y(lv), r, ink())
        layers(rv, look, grid, marks, good)
    }

    private fun layers(rv: RemoteViews, look: Look, grid: Bitmap, marks: Bitmap, good: Bitmap) {
        rv.setImageViewBitmap(R.id.chart_grid, grid); rv.setImageViewBitmap(R.id.chart_marks, marks); rv.setImageViewBitmap(R.id.chart_good, good)
        rv.setColorInt(R.id.chart_grid, "setColorFilter", look.muted.first, look.muted.second)
        rv.setColorInt(R.id.chart_marks, "setColorFilter", look.accent.first, look.accent.second)
        val ok = look.tone("ok")
        rv.setColorInt(R.id.chart_good, "setColorFilter", ok.first, ok.second)
    }
}
