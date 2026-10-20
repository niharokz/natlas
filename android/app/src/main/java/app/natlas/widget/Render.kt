package app.natlas.widget

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.res.ColorStateList
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.style.ForegroundColorSpan
import android.text.style.RelativeSizeSpan
import android.text.style.StyleSpan
import android.util.SizeF
import android.view.View
import android.widget.RemoteViews
import app.natlas.MainActivity
import app.natlas.Prefs
import app.natlas.QuickAddActivity
import app.natlas.R
import org.json.JSONObject
import java.time.DayOfWeek
import java.time.LocalDate
import java.time.LocalTime
import java.time.format.DateTimeFormatter
import java.time.format.TextStyle
import java.time.temporal.ChronoUnit
import java.util.Locale

// Draws every widget kind from the snapshot. Pure drawing: no engine calls here.
object Render {
    private data class Row(
        val header: String? = null, val key: String = "", val title: String = "", val sub: String = "",
        val badge: String = "", val tone: String = "", val dot: Int? = null,
        val tick: JSONObject? = null, val route: String = "#/today",
    )

    fun build(ctx: Context, kind: Kind, id: Int, snap: JSONObject?): RemoteViews {
        val look = Look(ctx)
        if (snap == null) return message(ctx, look, id, "Open Natlas once to load your lists")
        return try {
            when (kind) {
                Kind.TODAY -> today(ctx, look, id, snap)
                Kind.LIST -> list(ctx, look, id, snap)
                Kind.AGENDA -> RemoteViews(mapOf(SizeF(100f, 40f) to agendaSmall(ctx, look, id, snap), SizeF(180f, 100f) to agenda(ctx, look, id, snap)))
                Kind.MONTH -> month(ctx, look, id, snap)
                Kind.UPNEXT -> RemoteViews(mapOf(SizeF(100f, 40f) to upNext(ctx, look, id, snap, small = true), SizeF(100f, 100f) to upNext(ctx, look, id, snap, small = false)))
                Kind.QUICK -> RemoteViews(mapOf(SizeF(40f, 40f) to quick(ctx, look, id, snap, small = true), SizeF(180f, 40f) to quick(ctx, look, id, snap, small = false)))
                Kind.NUMBER -> number(ctx, look, id, snap)
                Kind.GLANCE -> glance(ctx, look, id, snap)
                Kind.CLOCK -> clock(ctx, look, id, snap)
                Kind.STEPS -> HealthWidgets.steps(ctx, look, id, snap) { r, c -> open(ctx, r, c) }
                Kind.SLEEP -> HealthWidgets.sleep(ctx, look, id, snap) { r, c -> open(ctx, r, c) }
                Kind.WEEK -> HealthWidgets.week(ctx, look, id, snap) { r, c -> open(ctx, r, c) }
                Kind.WEIGHT -> HealthWidgets.weight(ctx, look, id, snap) { r, c -> open(ctx, r, c) }
            }
        } catch (e: Exception) {
            message(ctx, look, id, "Widget error: ${e.message}")
        }
    }

    // ── shared pieces ──────────────────────────────────────────────────

    // Request codes must differ per widget and per button, or Android merges the intents.
    private fun code(id: Int, n: Int) = id * 1000 + n

    private fun open(ctx: Context, route: String, code: Int): PendingIntent =
        PendingIntent.getActivity(ctx, code, Intent(ctx, MainActivity::class.java)
            .putExtra(MainActivity.EXTRA_ROUTE, route)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

    private fun quickAdd(ctx: Context, source: String, code: Int): PendingIntent =
        PendingIntent.getActivity(ctx, code, Intent(ctx, QuickAddActivity::class.java)
            .putExtra(QuickAddActivity.EXTRA_SOURCE, source)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

    private fun broadcast(ctx: Context, action: String, id: Int, code: Int, extra: Int = 0): PendingIntent =
        PendingIntent.getBroadcast(ctx, code, Intent(ctx, WidgetReceiver::class.java).setAction(action)
            .putExtra("id", id).putExtra("delta", extra),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

    // Rows in a list send fill-in intents to WidgetRouter (tick / open).
    private fun template(ctx: Context, id: Int): PendingIntent =
        PendingIntent.getActivity(ctx, code(id, 990), Intent(ctx, WidgetRouter::class.java),
            PendingIntent.FLAG_MUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)

    private fun message(ctx: Context, look: Look, id: Int, text: String): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_message)
        look.background(rv, R.id.bg)
        rv.setTextViewText(R.id.text, text)
        look.text(rv, R.id.text, look.muted)
        rv.setOnClickPendingIntent(R.id.root, open(ctx, "#/today", code(id, 999)))
        return rv
    }

    private fun days(snap: JSONObject, iso: String): Long = try {
        ChronoUnit.DAYS.between(LocalDate.parse(snap.optString("today")), LocalDate.parse(iso))
    } catch (_: Exception) { 0 }

    private fun dayLabel(snap: JSONObject, iso: String): String {
        val n = days(snap, iso)
        return when (n) {
            0L -> "Today"; 1L -> "Tomorrow"; -1L -> "Yesterday"
            else -> try { LocalDate.parse(iso).format(DateTimeFormatter.ofPattern("EEE d MMM", Locale.getDefault())) } catch (_: Exception) { iso }
        }
    }

    private fun clock12(t: String): String = try {
        LocalTime.parse(t).format(DateTimeFormatter.ofPattern("h:mm a", Locale.getDefault())).lowercase()
    } catch (_: Exception) { t }

    // The list-type frame: header, undo bar, rows.
    private fun listFrame(
        ctx: Context, look: Look, id: Int, title: String, sub: String,
        headRoute: String, addSource: String?, rows: List<Row>, empty: String,
    ): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_list)
        look.background(rv, R.id.bg)
        rv.setTextViewText(R.id.title, title)
        rv.setTextViewText(R.id.sub, sub)
        look.text(rv, R.id.title, look.text)
        look.text(rv, R.id.sub, look.muted)
        rv.setColorInt(R.id.add, "setColorFilter", look.accent.first, look.accent.second)
        rv.setColorInt(R.id.refresh, "setColorFilter", look.muted.first, look.muted.second)
        rv.setOnClickPendingIntent(R.id.head, open(ctx, headRoute, code(id, 1)))
        rv.setOnClickPendingIntent(R.id.refresh, broadcast(ctx, WidgetReceiver.REFRESH, id, code(id, 2)))
        if (addSource != null) rv.setOnClickPendingIntent(R.id.add, quickAdd(ctx, addSource, code(id, 3)))
        else rv.setViewVisibility(R.id.add, View.GONE)

        // Undo bar for a few seconds after a tick on any widget
        val prefs = Prefs(ctx)
        if (prefs.undoUntil > System.currentTimeMillis() && prefs.undo.isNotEmpty()) {
            val u = JSONObject(prefs.undo)
            rv.setViewVisibility(R.id.undo_bar, View.VISIBLE)
            rv.setTextViewText(R.id.undo_text, "${u.optString("label", "Done")}: ${look.mask(u.optString("title"))}")
            rv.setColorStateList(R.id.undo_bar, "setBackgroundTintList", ColorStateList.valueOf(look.chip.first), ColorStateList.valueOf(look.chip.second))
            look.text(rv, R.id.undo_text, look.text)
            look.text(rv, R.id.undo_btn, look.accent)
            rv.setOnClickPendingIntent(R.id.undo_btn, PendingIntent.getActivity(ctx, code(id, 4),
                Intent(ctx, WidgetRouter::class.java).putExtra("do", "undo"),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
        }

        val items = RemoteViews.RemoteCollectionItems.Builder().setHasStableIds(true).setViewTypeCount(2)
        val box = ColorStateList.valueOf(look.accent.second)
        for ((i, r) in rows.withIndex()) {
            if (r.header != null) {
                val h = RemoteViews(ctx.packageName, R.layout.w_row_header)
                h.setTextViewText(R.id.h, r.header)
                look.text(h, R.id.h, look.muted)
                items.addItem(("h" + r.header + i).hashCode().toLong(), h)
                continue
            }
            val row = RemoteViews(ctx.packageName, R.layout.w_row)
            row.setTextViewText(R.id.row_title, look.mask(r.title))
            look.text(row, R.id.row_title, look.text)
            if (r.sub.isEmpty()) row.setViewVisibility(R.id.row_sub, View.GONE) else {
                row.setTextViewText(R.id.row_sub, r.sub)
                look.text(row, R.id.row_sub, if (r.tone == "overdue") look.tone("bad") else look.muted)
            }
            if (r.badge.isNotEmpty()) {
                row.setViewVisibility(R.id.row_badge, View.VISIBLE)
                row.setTextViewText(R.id.row_badge, r.badge)
                look.text(row, R.id.row_badge, look.tone(r.tone))
                row.setColorStateList(R.id.row_badge, "setBackgroundTintList", ColorStateList.valueOf(look.chip.first), ColorStateList.valueOf(look.chip.second))
            }
            if (r.dot != null) {
                row.setViewVisibility(R.id.dot, View.VISIBLE)
                row.setInt(R.id.dot, "setColorFilter", r.dot)
            }
            if (r.tick != null) {
                row.setViewVisibility(R.id.check, View.VISIBLE)
                row.setCompoundButtonChecked(R.id.check, false)
                row.setColorStateList(R.id.check, "setButtonTintList", box)
                row.setOnCheckedChangeResponse(R.id.check, RemoteViews.RemoteResponse.fromFillInIntent(
                    Intent().putExtra("do", "tick").putExtra("req", r.tick.toString())))
            } else if (r.dot == null) {
                row.setViewPadding(R.id.row, dp(ctx, 4), 0, dp(ctx, 6), 0)
            }
            row.setOnClickFillInIntent(R.id.row, Intent().putExtra("do", "open").putExtra("route", r.route))
            items.addItem((r.key + i).hashCode().toLong(), row)
        }
        rv.setRemoteAdapter(R.id.list, items.build())
        rv.setPendingIntentTemplate(R.id.list, template(ctx, id))
        rv.setTextViewText(R.id.empty, empty)
        look.text(rv, R.id.empty, look.muted)
        rv.setEmptyView(R.id.list, R.id.empty)
        return rv
    }

    private fun dp(ctx: Context, v: Int) = (v * ctx.resources.displayMetrics.density).toInt()

    private fun tickReq(plugin: String, coll: String, id: String, rev: String, action: JSONObject, title: String) =
        JSONObject().put("op", "action").put("plugin", plugin).put("coll", coll).put("id", id).put("rev", rev)
            .put("action", action.optString("id")).put("label", action.optString("label")).put("title", title)

    // ── Today ──────────────────────────────────────────────────────────

    private fun today(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rows = mutableListOf<Row>()
        val labels = mapOf("overdue" to "Overdue", "today" to "Today", "upcoming" to "Next 7 days")
        val items = Snap.objs(snap.optJSONArray("agenda"))
        for ((sec, label) in labels) {
            val inSec = items.filter { it.optString("section") == sec }
            if (inSec.isEmpty()) continue
            rows += Row(header = "$label · ${inSec.size}")
            for (it in inSec) {
                val plugin = it.optString("plugin"); val coll = it.optString("coll"); val rid = it.optString("id")
                val act = it.optJSONObject("action")
                rows += Row(key = "$plugin/$coll/$rid", title = it.optString("title"),
                    sub = listOf(it.optString("when").replaceFirstChar { c -> c.uppercase() }, Snap.pluginTitle(snap, plugin)).filter { s -> s.isNotEmpty() }.joinToString(" · "),
                    tone = if (sec == "overdue") "overdue" else "",
                    tick = act?.let { a -> tickReq(plugin, coll, rid, it.optString("rev"), a, it.optString("title")) },
                    route = "#/p/$plugin/$coll/r/$rid")
            }
        }
        val count = items.count { it.optString("section") != "upcoming" }
        val sub = if (count == 0) "Nothing due" else "$count due"
        val add = Snap.lists(snap, quickOnly = true).firstOrNull()?.first
        return listFrame(ctx, look, id, LocalDate.now().format(DateTimeFormatter.ofPattern("EEEE d MMMM", Locale.getDefault())),
            sub, "#/today", add, rows, "Nothing due. Enjoy the day.")
    }

    // ── List (any collection) ──────────────────────────────────────────

    private fun list(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val lists = Snap.objs(snap.optJSONArray("lists"))
        val src = Prefs(ctx).widgetSource(id)
        val l = lists.firstOrNull { "${it.optString("plugin")}/${it.optString("coll")}" == src }
            ?: lists.firstOrNull { it.optString("plugin") == "events" } ?: lists.firstOrNull()
            ?: return message(ctx, look, id, "No lists to show")
        val plugin = l.optString("plugin"); val coll = l.optString("coll")
        val rows = mutableListOf<Row>()
        var total = 0
        val groups = Snap.objs(l.optJSONArray("groups"))
        for (g in groups) {
            val items = Snap.objs(g.optJSONArray("items"))
            if (items.isEmpty()) continue
            total += items.size
            if (g.optString("label").isNotEmpty() && groups.size > 1) rows += Row(header = "${g.optString("label")} · ${items.size}")
            for (it in items) {
                val rid = it.optString("id")
                rows += Row(key = "$plugin/$coll/$rid", title = it.optString("title"), sub = it.optString("sub"),
                    badge = it.optString("badge"), tone = it.optString("tone"),
                    tick = it.optJSONObject("action")?.let { a -> tickReq(plugin, coll, rid, it.optString("rev"), a, it.optString("title")) },
                    route = "#/p/$plugin/$coll/r/$rid")
            }
        }
        val add = if (l.optBoolean("quick")) "$plugin/$coll" else null
        return listFrame(ctx, look, id, l.optString("title"), if (total == 0) "All clear" else "$total open",
            "#/p/$plugin/$coll", add, rows, "Nothing here.")
    }

    // ── Calendar: agenda + month ───────────────────────────────────────

    private fun upcomingEntries(snap: JSONObject): List<JSONObject> {
        val today = snap.optString("today")
        return Snap.objs(snap.optJSONArray("calendar")).filter { it.optString("end") >= today }
    }

    private fun calRoute(e: JSONObject, snap: JSONObject): String =
        if (e.optBoolean("readonly")) "#/p/${e.optString("plugin")}/day/${maxOf(e.optString("date"), snap.optString("today"))}"
        else "#/p/${e.optString("plugin")}/${e.optString("coll")}/r/${e.optString("id")}"

    private fun calPlugin(snap: JSONObject): JSONObject? =
        Snap.objs(snap.optJSONArray("plugins")).firstOrNull { it.optString("view") == "calendar" }

    private fun agenda(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rows = mutableListOf<Row>()
        var lastDay = ""
        val today = snap.optString("today")
        val entries = upcomingEntries(snap).take(60)
        for (e in entries) {
            val day = maxOf(e.optString("date"), today)
            if (day != lastDay) { rows += Row(header = dayLabel(snap, day)); lastDay = day }
            val time = e.optString("time")
            val sub = listOf(if (time.isEmpty()) "All day" else clock12(time), e.optString("cal")).joinToString(" · ")
            rows += Row(key = e.optString("coll") + "/" + e.optString("id"), title = e.optString("title"), sub = sub,
                dot = look.category(e.optString("cat")), route = calRoute(e, snap))
        }
        val p = calPlugin(snap)
        val pid = p?.optString("id") ?: "calendar"
        val add = Snap.lists(snap, quickOnly = true).firstOrNull { it.first.startsWith("$pid/") }?.first
        return listFrame(ctx, look, id, p?.optString("title") ?: "Calendar", "${entries.size} coming up",
            "#/p/$pid", add, rows, "Nothing in the next two months.")
    }

    private fun agendaSmall(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_agenda_small)
        look.background(rv, R.id.bg)
        val e = upcomingEntries(snap).firstOrNull()
        if (e == null) {
            rv.setTextViewText(R.id.title, "Nothing coming up")
            rv.setViewVisibility(R.id.dot, View.GONE)
        } else {
            rv.setTextViewText(R.id.title, look.mask(e.optString("title")))
            val time = e.optString("time")
            rv.setTextViewText(R.id.sub, listOf(if (time.isEmpty()) "All day" else clock12(time), e.optString("cal")).joinToString(" · "))
            rv.setTextViewText(R.id.when_text, dayLabel(snap, maxOf(e.optString("date"), snap.optString("today"))))
            rv.setInt(R.id.dot, "setColorFilter", look.category(e.optString("cat")))
        }
        look.text(rv, R.id.title, look.text); look.text(rv, R.id.sub, look.muted); look.text(rv, R.id.when_text, look.accent)
        rv.setOnClickPendingIntent(R.id.root, open(ctx, e?.let { calRoute(it, snap) } ?: "#/p/calendar", code(id, 9)))
        return rv
    }

    private fun month(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_month)
        look.background(rv, R.id.bg)
        val offset = Prefs(ctx).monthOffset(id)
        val today = try { LocalDate.parse(snap.optString("today")) } catch (_: Exception) { LocalDate.now() }
        val first = today.withDayOfMonth(1).plusMonths(offset.toLong())
        val p = calPlugin(snap)
        val pid = p?.optString("id") ?: "calendar"
        val weekStart = if (p?.optString("week_start") == "sunday") DayOfWeek.SUNDAY else DayOfWeek.MONDAY
        rv.setTextViewText(R.id.title, first.format(DateTimeFormatter.ofPattern("MMMM yyyy", Locale.getDefault())))
        look.text(rv, R.id.title, look.text)
        for (iv in listOf(R.id.prev, R.id.next)) rv.setColorInt(iv, "setColorFilter", look.muted.first, look.muted.second)
        rv.setOnClickPendingIntent(R.id.prev, broadcast(ctx, WidgetReceiver.MONTH, id, code(id, 161), -1))
        rv.setOnClickPendingIntent(R.id.next, broadcast(ctx, WidgetReceiver.MONTH, id, code(id, 162), 1))
        rv.setOnClickPendingIntent(R.id.title, broadcast(ctx, WidgetReceiver.MONTH, id, code(id, 163), 0))
        for (i in 0 until 7) {
            val dow = weekStart.plus(i.toLong())
            rv.setTextViewText(MonthIds.weekdays[i], dow.getDisplayName(TextStyle.SHORT, Locale.getDefault()).take(2))
            look.text(rv, MonthIds.weekdays[i], look.muted)
        }
        // colours per day (multi-day entries on every day they cover)
        val byDay = HashMap<LocalDate, MutableList<Int>>()
        for (e in Snap.objs(snap.optJSONArray("calendar"))) {
            val s = try { LocalDate.parse(e.optString("date")) } catch (_: Exception) { continue }
            val end = try { LocalDate.parse(e.optString("end")) } catch (_: Exception) { s }
            var d = s
            var guard = 0
            while (!d.isAfter(end) && guard++ < 60) { byDay.getOrPut(d) { mutableListOf() } += look.category(e.optString("cat")); d = d.plusDays(1) }
        }
        val shift = ((first.dayOfWeek.value - weekStart.value) + 7) % 7
        val start = first.minusDays(shift.toLong())
        val weeks = ((shift + first.lengthOfMonth() + 6) / 7)
        for (r in 0 until 6) rv.setViewVisibility(MonthIds.rows[r], if (r < weeks) View.VISIBLE else View.GONE)
        for (i in 0 until 42) {
            val cell = MonthIds.cells[i]
            val day = start.plusDays(i.toLong())
            val inMonth = day.month == first.month
            val sb = SpannableStringBuilder(day.dayOfMonth.toString())
            if (day == today) sb.setSpan(StyleSpan(android.graphics.Typeface.BOLD), 0, sb.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            val colours = byDay[day].orEmpty().distinct()
            sb.append("\n")
            if (colours.isEmpty()) sb.append(" ") else for (c in colours.take(3)) {
                val at = sb.length
                sb.append("•")
                sb.setSpan(ForegroundColorSpan(c), at, sb.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
                sb.setSpan(RelativeSizeSpan(1.25f), at, sb.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            }
            rv.setTextViewText(cell, sb)
            when {
                day == today -> {
                    rv.setInt(cell, "setBackgroundResource", R.drawable.widget_today)
                    look.text(rv, cell, look.accent)
                }
                else -> {
                    rv.setInt(cell, "setBackgroundResource", R.drawable.row_ripple)
                    look.text(rv, cell, if (inMonth) look.text else look.line)
                }
            }
            rv.setOnClickPendingIntent(cell, open(ctx, "#/p/$pid/day/$day", code(id, 100 + i)))
        }
        return rv
    }

    // ── Up next ────────────────────────────────────────────────────────

    private fun upNext(ctx: Context, look: Look, id: Int, snap: JSONObject, small: Boolean): RemoteViews {
        val rv = RemoteViews(ctx.packageName, if (small) R.layout.w_upnext_small else R.layout.w_upnext)
        look.background(rv, R.id.bg)
        val now = LocalTime.now().toString().take(5)
        val today = snap.optString("today")
        val cal = upcomingEntries(snap).firstOrNull { e ->
            val d = e.optString("date")
            d > today || e.optString("time").isEmpty() || e.optString("time") >= now || e.optString("end") > today
        }
        val agenda = Snap.objs(snap.optJSONArray("agenda")).firstOrNull { it.optString("section") != "overdue" }
        val title: String; val iso: String; val sub: String; val route: String
        when {
            cal != null -> {
                title = cal.optString("title"); iso = maxOf(cal.optString("date"), today)
                sub = listOf(dayLabel(snap, iso), cal.optString("time").let { if (it.isEmpty()) "" else clock12(it) }, cal.optString("cal")).filter { it.isNotEmpty() }.joinToString(" · ")
                route = calRoute(cal, snap)
            }
            agenda != null -> {
                title = agenda.optString("title"); iso = ""
                sub = agenda.optString("when").replaceFirstChar { it.uppercase() } + " · " + Snap.pluginTitle(snap, agenda.optString("plugin"))
                route = "#/p/${agenda.optString("plugin")}/${agenda.optString("coll")}/r/${agenda.optString("id")}"
            }
            else -> { title = "Nothing coming up"; iso = ""; sub = ""; route = "#/today" }
        }
        val n = if (iso.isEmpty()) agenda?.optInt("days", 0)?.toLong() ?: 0L else days(snap, iso)
        val (big, unit) = when {
            cal == null && agenda == null -> "✓" to ""
            n <= 0L -> "Today" to ""
            n == 1L -> "1" to "day"
            else -> n.toString() to "days"
        }
        rv.setTextViewText(R.id.big, big)
        if (!small) rv.setTextViewText(R.id.unit, unit)
        rv.setTextViewText(R.id.title, look.mask(title))
        rv.setTextViewText(R.id.sub, sub)
        look.text(rv, R.id.big, look.accent); look.text(rv, R.id.title, look.text); look.text(rv, R.id.sub, look.muted)
        if (!small) { look.text(rv, R.id.unit, look.muted); look.text(rv, R.id.label, look.muted) }
        rv.setOnClickPendingIntent(R.id.root, open(ctx, route, code(id, if (small) 21 else 22)))
        return rv
    }

    // ── Quick add ──────────────────────────────────────────────────────

    private fun quickSource(ctx: Context, id: Int, snap: JSONObject): Pair<String, String> {
        val options = Snap.lists(snap, quickOnly = true)
        val src = Prefs(ctx).widgetSource(id)
        return options.firstOrNull { it.first == src } ?: options.firstOrNull { it.first.startsWith("events/") } ?: options.firstOrNull() ?: ("" to "")
    }

    private fun quick(ctx: Context, look: Look, id: Int, snap: JSONObject, small: Boolean): RemoteViews {
        val rv = RemoteViews(ctx.packageName, if (small) R.layout.w_quick_small else R.layout.w_quick)
        look.background(rv, R.id.bg)
        val (src, label) = quickSource(ctx, id, snap)
        rv.setInt(R.id.add, "setColorFilter", look.onAccent.second)
        rv.setColorStateList(R.id.add, "setBackgroundTintList", ColorStateList.valueOf(look.accent.second))
        if (!small) {
            rv.setTextViewText(R.id.hint, "Add to $label…")
            look.text(rv, R.id.hint, look.muted)
            rv.setColorInt(R.id.icon, "setColorFilter", look.accent.first, look.accent.second)
        }
        val pi = quickAdd(ctx, src, code(id, if (small) 5 else 15))
        rv.setOnClickPendingIntent(R.id.root, pi)
        rv.setOnClickPendingIntent(R.id.add, pi)
        return rv
    }

    // ── Numbers ────────────────────────────────────────────────────────

    private fun statKey(s: JSONObject) = "${s.optString("plugin")}|${s.optString("coll")}|${s.optString("label")}"

    private fun number(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_number)
        look.background(rv, R.id.bg)
        val stats = Snap.objs(snap.optJSONArray("stats"))
        val src = Prefs(ctx).widgetSource(id)
        val s = stats.firstOrNull { statKey(it) == src } ?: stats.firstOrNull { it.optString("tone") == "warn" } ?: stats.firstOrNull()
            ?: return message(ctx, look, id, "No numbers yet")
        rv.setTextViewText(R.id.sub, s.optString("title"))
        rv.setTextViewText(R.id.value, if (look.privacy) "•••" else s.optString("value"))
        rv.setTextViewText(R.id.label, s.optString("label"))
        val tone = s.optString("tone")
        look.text(rv, R.id.value, if (tone.isNotEmpty() && s.optDouble("raw", 0.0) != 0.0) look.tone(tone) else look.text)
        look.text(rv, R.id.sub, look.muted); look.text(rv, R.id.label, look.muted)
        rv.setOnClickPendingIntent(R.id.root, open(ctx, "#/p/${s.optString("plugin")}/${s.optString("coll")}", code(id, 6)))
        return rv
    }

    private fun glance(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_glance)
        look.background(rv, R.id.bg)
        val stats = Snap.objs(snap.optJSONArray("stats"))
        val src = Prefs(ctx).widgetSource(id)
        val plugin = if (stats.any { it.optString("plugin") == src }) src else stats.firstOrNull()?.optString("plugin") ?: ""
        val mine = stats.filter { it.optString("plugin") == plugin }.take(4)
        rv.setTextViewText(R.id.title, Snap.pluginTitle(snap, plugin))
        look.text(rv, R.id.title, look.muted)
        val cells = listOf(R.id.g0, R.id.g1, R.id.g2, R.id.g3)
        val values = listOf(R.id.v0, R.id.v1, R.id.v2, R.id.v3)
        val labels = listOf(R.id.l0, R.id.l1, R.id.l2, R.id.l3)
        for (i in 0 until 4) {
            val s = mine.getOrNull(i)
            if (s == null) { rv.setViewVisibility(cells[i], View.GONE); continue }
            rv.setTextViewText(values[i], if (look.privacy) "•••" else s.optString("value"))
            rv.setTextViewText(labels[i], s.optString("label"))
            val tone = s.optString("tone")
            look.text(rv, values[i], if (tone.isNotEmpty() && s.optDouble("raw", 0.0) != 0.0) look.tone(tone) else look.text)
            look.text(rv, labels[i], look.muted)
            rv.setOnClickPendingIntent(cells[i], open(ctx, "#/p/$plugin/${s.optString("coll")}", code(id, 7 + i)))
        }
        return rv
    }

    // ── Clock + agenda ─────────────────────────────────────────────────

    private fun clock(ctx: Context, look: Look, id: Int, snap: JSONObject): RemoteViews {
        val rv = RemoteViews(ctx.packageName, R.layout.w_clock)
        look.background(rv, R.id.bg)
        look.text(rv, R.id.time, look.text); look.text(rv, R.id.date, look.accent)
        rv.setOnClickPendingIntent(R.id.head, open(ctx, "#/today", code(id, 31)))
        // next three: calendar today/ahead and the agenda, soonest first
        data class Item(val title: String, val whenText: String, val dot: Int, val sort: String, val route: String)
        val today = snap.optString("today")
        val list = mutableListOf<Item>()
        for (e in upcomingEntries(snap).take(6)) {
            val d = maxOf(e.optString("date"), today)
            val t = e.optString("time")
            list += Item(e.optString("title"), if (d == today && t.isNotEmpty()) clock12(t) else dayLabel(snap, d),
                look.category(e.optString("cat")), d + (t.ifEmpty { "00:00" }), calRoute(e, snap))
        }
        for (a in Snap.objs(snap.optJSONArray("agenda")).filter { it.optString("section") != "upcoming" }.take(6)) {
            list += Item(a.optString("title"), a.optString("when").replaceFirstChar { it.uppercase() }, look.accent.second,
                today + (if (a.optString("section") == "overdue") "!" else "0"),
                "#/p/${a.optString("plugin")}/${a.optString("coll")}/r/${a.optString("id")}")
        }
        val top = list.sortedBy { it.sort }.take(3)
        val rows = listOf(R.id.a0, R.id.a1, R.id.a2)
        val titles = listOf(R.id.at0, R.id.at1, R.id.at2)
        val whens = listOf(R.id.aw0, R.id.aw1, R.id.aw2)
        val dots = listOf(R.id.ad0, R.id.ad1, R.id.ad2)
        for (i in 0 until 3) {
            val it = top.getOrNull(i)
            if (it == null) { rv.setViewVisibility(rows[i], View.GONE); continue }
            rv.setTextViewText(titles[i], look.mask(it.title))
            rv.setTextViewText(whens[i], it.whenText)
            rv.setInt(dots[i], "setColorFilter", it.dot)
            look.text(rv, titles[i], look.text); look.text(rv, whens[i], look.muted)
            rv.setOnClickPendingIntent(rows[i], open(ctx, it.route, code(id, 32 + i)))
        }
        if (top.isEmpty()) {
            rv.setViewVisibility(R.id.empty, View.VISIBLE)
            rv.setTextViewText(R.id.empty, "Nothing due. Enjoy the day.")
            look.text(rv, R.id.empty, look.muted)
        }
        return rv
    }
}
