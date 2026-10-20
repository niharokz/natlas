package app.natlas.health

import android.annotation.TargetApi
import android.app.job.JobInfo
import android.app.job.JobParameters
import android.app.job.JobScheduler
import android.app.job.JobService
import android.content.ComponentName
import android.content.Context
import android.os.Build
import app.natlas.Engine
import app.natlas.Prefs
import app.natlas.Refresh
import app.natlas.widget.Snap
import org.json.JSONArray
import org.json.JSONObject
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZonedDateTime
import java.util.concurrent.atomic.AtomicBoolean

// Health Connect → health.md. Every morning at the set time (Settings → Health)
// the last 3 full days and last night's sleep are written; "Import history" walks
// back through everything Health Connect has. Writes go through `natlas cli`
// (op upsert), so a day that did not change is not rewritten.
// Every entry point checks supported() (Android 14+) before touching Health Connect.
@TargetApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
object HealthSync {
    private const val JOB = 77
    private val running = AtomicBoolean(false)

    fun supported() = Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE

    // The list to fill: the collection with sync: health_connect ("health/daily").
    fun target(ctx: Context): Pair<String, String>? {
        var snap = Snap.load(ctx)
        if (snap == null) { Refresh.now(ctx); snap = Snap.load(ctx) }
        for (p in Snap.objs(snap?.optJSONArray("plugins"))) for (c in Snap.objs(p.optJSONArray("collections")))
            if (c.optString("sync") == "health_connect") return p.optString("id") to c.optString("id")
        return null
    }

    // Morning sync (also "Sync now"). Returns a short note for Settings.
    fun runDaily(ctx: Context): String {
        if (!supported()) return note(ctx, "Needs Android 14 or newer")
        if (!running.compareAndSet(false, true)) return "A sync is already running"
        try {
            val today = LocalDate.now()
            val res = readAndWrite(ctx, today.minusDays(3), today, pointsOnly = today)
            if (res.ok) Prefs(ctx).healthDay = today.toString()
            return note(ctx, res.text)
        } finally { running.set(false) }
    }

    // Every day back to the first one Health Connect has (90 days at a time,
    // stops after a year with nothing). progress(text) is called after each step.
    fun importHistory(ctx: Context, progress: (String) -> Unit): String {
        if (!supported()) return note(ctx, "Needs Android 14 or newer")
        if (!running.compareAndSet(false, true)) return "A sync is already running"
        try {
            var to = LocalDate.now().minusDays(1)
            var empty = 0
            var total = 0
            while (empty < 4 && to.year >= 2010) {
                val from = to.minusDays(89)
                progress("Reading ${from} – ${to}… ($total days so far)")
                val res = readAndWrite(ctx, from, to, pointsOnly = null, refresh = false)
                if (!res.ok) return note(ctx, res.text)
                if (res.days == 0) empty++ else { empty = 0; total += res.days }
                to = from.minusDays(1)
            }
            Prefs(ctx).healthImported = true
            Thread { Refresh.now(ctx) }.start()
            val hist = if (HealthConnect.granted(ctx, HealthConnect.HISTORY)) "" else " (only the last 30 days: allow \"past data\" for more)"
            return note(ctx, "Imported $total days$hist")
        } finally { running.set(false) }
    }

    private class Result(val ok: Boolean, val days: Int, val text: String)

    private fun readAndWrite(ctx: Context, from: LocalDate, to: LocalDate, pointsOnly: LocalDate?, refresh: Boolean = true): Result {
        if (HealthConnect.grantedCount(ctx) == 0) return Result(false, 0, "No Health Connect permission yet")
        val (plugin, coll) = target(ctx) ?: return Result(false, 0, "Turn on the Health list (Settings → Lists) first")
        val reader = try { HealthReader(ctx) } catch (e: Exception) { return Result(false, 0, e.message ?: "Health Connect not available") }
        val days = try { reader.read(from, to, pointsOnly) } catch (e: Exception) {
            return Result(false, 0, "Health Connect: " + (e.message ?: e.javaClass.simpleName))
        } finally { reader.close() }
        if (days.isEmpty()) return Result(true, 0, "No new data from Health Connect")
        val req = JSONObject().put("op", "upsert").put("plugin", plugin).put("coll", coll).put("records", JSONArray(days))
        val out = Engine.cli(ctx, req)
        if (out.optInt("status") != 200) return Result(false, 0, "Could not save: " + out.optString("error", "unknown error"))
        if (reader.stepSources.isNotEmpty() && pointsOnly != null) Prefs(ctx).healthSources = reader.stepSources
        if (refresh) Refresh.now(ctx)
        val added = out.optInt("added"); val changed = out.optInt("changed")
        val skipped = if (reader.skipped.isEmpty()) "" else " · not allowed: " + reader.skipped.joinToString(", ")
        return Result(true, days.size, when {
            added + changed == 0 -> "Up to date ($from – $to)"
            else -> "$added new, $changed updated day${if (added + changed == 1) "" else "s"}"
        } + skipped)
    }

    private fun note(ctx: Context, text: String): String {
        val p = Prefs(ctx)
        p.healthNote = text
        p.healthLast = System.currentTimeMillis()
        return text
    }

    // ── schedule: one job a day at the set time (± up to an hour, Android decides)

    fun schedule(ctx: Context) {
        val js = ctx.getSystemService(JobScheduler::class.java) ?: return
        val prefs = Prefs(ctx)
        if (!prefs.healthOn || !supported()) { js.cancel(JOB); return }
        val at = LocalTime.parse(prefs.healthTime)
        var next = ZonedDateTime.now().with(at).withSecond(0)
        if (!next.isAfter(ZonedDateTime.now())) next = next.plusDays(1)
        val delay = next.toInstant().toEpochMilli() - System.currentTimeMillis()
        js.schedule(JobInfo.Builder(JOB, ComponentName(ctx, HealthJob::class.java))
            .setMinimumLatency(delay)
            .setOverrideDeadline(delay + 60 * 60 * 1000L)
            .setPersisted(true)
            .build())
    }

    // Opening the app after a missed morning sync (phone off, no background permission).
    fun catchUp(ctx: Context) {
        val prefs = Prefs(ctx)
        if (!prefs.healthOn || !supported()) return
        val due = LocalTime.now().isAfter(LocalTime.parse(prefs.healthTime))
        if (due && prefs.healthDay != LocalDate.now().toString()) Thread { runDaily(ctx.applicationContext) }.start()
    }
}

// The morning job: sync, then schedule tomorrow's.
class HealthJob : JobService() {
    override fun onStartJob(params: JobParameters): Boolean {
        val app = applicationContext
        Thread {
            try { HealthSync.runDaily(app) } finally {
                HealthSync.schedule(app)
                jobFinished(params, false)
            }
        }.start()
        return true
    }

    override fun onStopJob(params: JobParameters): Boolean = true // stopped early: try again
}
