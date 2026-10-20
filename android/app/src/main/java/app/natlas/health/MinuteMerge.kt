package app.natlas.health

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

// Health Connect keeps steps (distance, calories, floors) per app and per device:
// one walk can arrive from the phone, the band and Google Fit, and its own daily
// total adds them up (19k walked → 35k). Here each minute counts once: the highest
// value any single source has for that minute, the way Samsung Health merges.
class MinuteMerge(private val zone: ZoneId) {
    class Part(val source: String, val start: Instant, val end: Instant, val value: Double)

    private val minutes = HashMap<Long, HashMap<String, Double>>() // epoch minute → source → value
    private val bySource = HashMap<String, HashMap<LocalDate, Double>>() // raw totals, for the breakdown

    // Spreads a record evenly over the minutes it covers (partial minutes by share).
    fun add(p: Part) {
        if (p.value <= 0) return
        val s = p.start.toEpochMilli()
        val e = maxOf(p.end.toEpochMilli(), s)
        bySource.getOrPut(p.source) { HashMap() }.merge(day(s), p.value, Double::plus)
        if (e == s) { put(s / 60_000, p.source, p.value); return }
        val total = (e - s).toDouble()
        var m = Math.floorDiv(s, 60_000L)
        while (m * 60_000 < e) {
            val from = maxOf(s, m * 60_000); val to = minOf(e, (m + 1) * 60_000)
            if (to > from) put(m, p.source, p.value * (to - from) / total)
            m++
        }
    }

    private fun put(minute: Long, source: String, v: Double) {
        minutes.getOrPut(minute) { HashMap(4) }.merge(source, v, Double::plus)
    }

    private fun day(millis: Long): LocalDate = Instant.ofEpochMilli(millis).atZone(zone).toLocalDate()

    // Per local day: the sum over minutes of the best source in each minute.
    fun days(): Map<LocalDate, Double> {
        val out = HashMap<LocalDate, Double>()
        for ((m, sources) in minutes) out.merge(day(m * 60_000), sources.values.max(), Double::plus)
        return out
    }

    // What each source counted on one day, largest first.
    fun sources(d: LocalDate): List<Pair<String, Double>> =
        bySource.mapNotNull { (src, days) -> days[d]?.let { src to it } }.sortedByDescending { it.second }
}
