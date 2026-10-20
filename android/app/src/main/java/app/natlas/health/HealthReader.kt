package app.natlas.health

import android.annotation.TargetApi
import android.content.Context
import android.content.pm.PackageManager
import android.health.connect.AggregateRecordsGroupedByPeriodResponse
import android.health.connect.AggregateRecordsRequest
import android.health.connect.AggregateRecordsResponse
import android.health.connect.HealthConnectException
import android.health.connect.HealthConnectManager
import android.health.connect.HealthPermissions
import android.health.connect.LocalTimeRangeFilter
import android.health.connect.ReadRecordsRequestUsingFilters
import android.health.connect.ReadRecordsResponse
import android.health.connect.TimeInstantRangeFilter
import android.health.connect.datatypes.ActiveCaloriesBurnedRecord
import android.health.connect.datatypes.AggregationType
import android.health.connect.datatypes.BloodGlucoseRecord
import android.health.connect.datatypes.BloodPressureRecord
import android.health.connect.datatypes.BodyFatRecord
import android.health.connect.datatypes.Device
import android.health.connect.datatypes.DistanceRecord
import android.health.connect.datatypes.ElevationGainedRecord
import android.health.connect.datatypes.ExerciseSessionRecord
import android.health.connect.datatypes.ExerciseSessionType
import android.health.connect.datatypes.FloorsClimbedRecord
import android.health.connect.datatypes.HeartRateRecord
import android.health.connect.datatypes.HeartRateVariabilityRmssdRecord
import android.health.connect.datatypes.HeightRecord
import android.health.connect.datatypes.HydrationRecord
import android.health.connect.datatypes.IntervalRecord
import android.health.connect.datatypes.NutritionRecord
import android.health.connect.datatypes.OxygenSaturationRecord
import android.health.connect.datatypes.Record
import android.health.connect.datatypes.RespiratoryRateRecord
import android.health.connect.datatypes.RestingHeartRateRecord
import android.health.connect.datatypes.SleepSessionRecord
import android.health.connect.datatypes.StepsRecord
import android.health.connect.datatypes.TotalCaloriesBurnedRecord
import android.health.connect.datatypes.WeightRecord
import android.os.Build
import android.os.OutcomeReceiver
import org.json.JSONObject
import java.io.IOException
import java.time.Duration
import java.time.Instant
import java.time.LocalDate
import java.time.Period
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.math.roundToInt

// Health Connect (built into Android 14+). Samsung Health, Strava and the band
// write into it; we only read. Field names match plugins/health/plugin.yml.
@TargetApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
object HealthConnect {
    const val BACKGROUND = "android.permission.health.READ_HEALTH_DATA_IN_BACKGROUND" // Android 15+
    const val HISTORY = "android.permission.health.READ_HEALTH_DATA_HISTORY" // older than 30 days

    val READ = listOf(
        HealthPermissions.READ_STEPS, HealthPermissions.READ_DISTANCE, HealthPermissions.READ_FLOORS_CLIMBED,
        HealthPermissions.READ_ELEVATION_GAINED, HealthPermissions.READ_ACTIVE_CALORIES_BURNED,
        HealthPermissions.READ_TOTAL_CALORIES_BURNED, HealthPermissions.READ_EXERCISE, HealthPermissions.READ_SLEEP,
        HealthPermissions.READ_HEART_RATE, HealthPermissions.READ_RESTING_HEART_RATE,
        HealthPermissions.READ_HEART_RATE_VARIABILITY, HealthPermissions.READ_OXYGEN_SATURATION,
        HealthPermissions.READ_RESPIRATORY_RATE, HealthPermissions.READ_WEIGHT, HealthPermissions.READ_BODY_FAT,
        HealthPermissions.READ_HEIGHT, HealthPermissions.READ_BLOOD_PRESSURE, HealthPermissions.READ_BLOOD_GLUCOSE,
        HealthPermissions.READ_HYDRATION, HealthPermissions.READ_NUTRITION,
    )
    val ALL = READ + BACKGROUND + HISTORY

    fun granted(ctx: Context, perm: String) = ctx.checkSelfPermission(perm) == PackageManager.PERMISSION_GRANTED
    fun grantedCount(ctx: Context) = READ.count { granted(ctx, it) }
}

// Reads days from Health Connect into plain JSON records ({date, steps, sleep_h, …}).
// Missing values are left out, never written as 0. Call off the UI thread.
@TargetApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
class HealthReader(private val ctx: Context) {
    private val hc: HealthConnectManager = ctx.getSystemService(HealthConnectManager::class.java)
        ?: throw IOException("Health Connect is not available on this phone")
    private val exec = Executors.newSingleThreadExecutor()
    private val zone = ZoneId.systemDefault()
    private val hhmm = DateTimeFormatter.ofPattern("HH:mm")
    val origins = sortedSetOf<String>() // apps the data came from
    val skipped = sortedSetOf<String>() // metrics not read (no permission, error)
    var stepSources = "" // per-source steps on the last full day, for Settings

    fun close() = exec.shutdown()

    // ── plumbing

    // Waits for one Health Connect call; retries when the hourly read quota is hit.
    private fun <T> await(call: (OutcomeReceiver<T, HealthConnectException>) -> Unit): T {
        var tries = 0
        while (true) {
            val latch = CountDownLatch(1)
            var out: T? = null
            var err: HealthConnectException? = null
            call(object : OutcomeReceiver<T, HealthConnectException> {
                override fun onResult(result: T) { out = result; latch.countDown() }
                override fun onError(error: HealthConnectException) { err = error; latch.countDown() }
            })
            if (!latch.await(90, TimeUnit.SECONDS)) throw IOException("Health Connect did not answer")
            val e = err
            if (e == null) {
                @Suppress("UNCHECKED_CAST")
                return out as T
            }
            if (e.errorCode == HealthConnectException.ERROR_RATE_LIMIT_EXCEEDED && tries++ < 8) {
                Thread.sleep(30_000L * tries) // Health Connect limits reads per hour; wait and go on
                continue
            }
            throw e
        }
    }

    // One value per local day.
    private fun <T> daily(type: AggregationType<T>, from: LocalDate, to: LocalDate): Map<LocalDate, T> {
        val filter = LocalTimeRangeFilter.Builder().setStartTime(from.atStartOfDay()).setEndTime(to.plusDays(1).atStartOfDay()).build()
        val req = AggregateRecordsRequest.Builder<T>(filter).addAggregationType(type).build()
        val res: List<AggregateRecordsGroupedByPeriodResponse<T>> = await { hc.aggregateGroupByPeriod(req, Period.ofDays(1), exec, it) }
        val out = HashMap<LocalDate, T>()
        for (r in res) r.get(type)?.let { out[r.startTime.toLocalDate()] = it }
        return out
    }

    // One value over an exact time window (a workout's distance).
    private fun <T> total(type: AggregationType<T>, start: Instant, end: Instant): T? {
        val filter = TimeInstantRangeFilter.Builder().setStartTime(start).setEndTime(end).build()
        val req = AggregateRecordsRequest.Builder<T>(filter).addAggregationType(type).build()
        val res: AggregateRecordsResponse<T> = await { hc.aggregate(req, exec, it) }
        return res.get(type)
    }

    private fun <T : Record> records(cls: Class<T>, start: Instant, end: Instant): List<T> {
        val out = ArrayList<T>()
        var token = -1L
        do {
            val b = ReadRecordsRequestUsingFilters.Builder(cls)
                .setTimeRangeFilter(TimeInstantRangeFilter.Builder().setStartTime(start).setEndTime(end).build())
                .setPageSize(5000)
            if (token != -1L) b.setPageToken(token)
            val res: ReadRecordsResponse<T> = await { hc.readRecords(b.build(), exec, it) }
            out += res.records
            res.records.forEach { origin(it) }
            token = res.nextPageToken
        } while (token != -1L)
        return out
    }

    // Interval records → each minute counted once, from its best source (app + device).
    private fun <R : IntervalRecord> merged(cls: Class<R>, start: Instant, end: Instant, value: (R) -> Double): MinuteMerge {
        val mm = MinuteMerge(zone)
        for (r in records(cls, start, end)) mm.add(MinuteMerge.Part(source(r), r.startTime, r.endTime, value(r)))
        return mm
    }

    // "Samsung Health · Galaxy Fit3", "Google Fit · phone"
    private fun source(r: Record): String {
        val pkg = r.metadata.dataOrigin.packageName ?: "?"
        val app = APPS[pkg] ?: pkg.substringAfterLast('.')
        val d = r.metadata.device
        val dev = d.model?.takeIf { it.isNotBlank() } ?: d.manufacturer?.takeIf { it.isNotBlank() } ?: when (d.type) {
            Device.DEVICE_TYPE_PHONE -> "phone"; Device.DEVICE_TYPE_WATCH -> "watch"; Device.DEVICE_TYPE_FITNESS_BAND -> "band"
            Device.DEVICE_TYPE_RING -> "ring"; else -> "device"
        }
        return "$app · $dev"
    }

    private fun origin(r: Record) {
        val pkg = r.metadata.dataOrigin.packageName ?: return
        origins += APPS[pkg] ?: pkg.substringAfterLast('.').replaceFirstChar { it.uppercase() }
    }

    // A metric that fails (usually: permission not granted) is skipped, the rest go on.
    private inline fun metric(name: String, block: () -> Unit) {
        try { block() } catch (e: HealthConnectException) {
            skipped += name
        } catch (e: SecurityException) {
            skipped += name
        }
    }

    private fun day(days: MutableMap<LocalDate, JSONObject>, d: LocalDate): JSONObject =
        days.getOrPut(d) { JSONObject().put("date", d.toString()) }

    private fun r1(v: Double) = (v * 10).roundToInt() / 10.0
    private fun r2(v: Double) = (v * 100).roundToInt() / 100.0
    private fun localDate(t: Instant) = t.atZone(zone).toLocalDate()

    // ── reading

    // Every metric for each day from..to. `pointsOnly` (today, read in the morning)
    // gets only last night's sleep and morning measurements, not day totals.
    fun read(from: LocalDate, to: LocalDate, pointsOnly: LocalDate? = null): List<JSONObject> {
        val days = sortedMapOf<LocalDate, JSONObject>()
        val totalsTo = if (pointsOnly != null && !to.isBefore(pointsOnly)) pointsOnly.minusDays(1) else to
        if (!totalsTo.isBefore(from)) totals(days, from, totalsTo)
        val start = from.minusDays(1).atTime(18, 0).atZone(zone).toInstant()
        val end = minOf(to.plusDays(1).atStartOfDay(zone).toInstant(), Instant.now())
        if (start.isBefore(end)) {
            sleep(days, from, to, start, end)
            points(days, from, to, start, end)
            if (!totalsTo.isBefore(from)) workouts(days, from, totalsTo)
        }
        for ((_, o) in days) if (origins.isNotEmpty()) o.put("sources", origins.joinToString(", "))
        return days.values.filter { it.length() > 2 } // more than date + sources
    }

    private fun totals(days: MutableMap<LocalDate, JSONObject>, from: LocalDate, to: LocalDate) {
        // counted per minute across phone, band and apps (MinuteMerge), not Health Connect's sums
        val start = from.atStartOfDay(zone).toInstant(); val end = to.plusDays(1).atStartOfDay(zone).toInstant()
        metric("steps") {
            val mm = merged(StepsRecord::class.java, start, end) { it.count.toDouble() }
            mm.days().forEach { (d, v) -> if (v >= 1) day(days, d).put("steps", v.roundToInt()) }
            val src = mm.sources(to)
            if (src.size > 1) stepSources = src.joinToString(" · ") { (k, v) -> "$k ${"%,d".format(v.roundToInt())}" } +
                " → counted ${"%,d".format((mm.days()[to] ?: 0.0).roundToInt())} on $to"
            else if (src.size == 1) stepSources = "${src[0].first} only"
        }
        metric("distance") { merged(DistanceRecord::class.java, start, end) { it.distance.inMeters }.days().forEach { (d, v) -> if (v > 0) day(days, d).put("km", r2(v / 1000)) } }
        metric("floors") { merged(FloorsClimbedRecord::class.java, start, end) { it.floors }.days().forEach { (d, v) -> if (v >= 0.5) day(days, d).put("floors", v.roundToInt()) } }
        metric("elevation") { merged(ElevationGainedRecord::class.java, start, end) { it.elevation.inMeters }.days().forEach { (d, v) -> if (v >= 0.5) day(days, d).put("elev_m", v.roundToInt()) } }
        metric("active calories") { merged(ActiveCaloriesBurnedRecord::class.java, start, end) { it.energy.inCalories / 1000 }.days().forEach { (d, v) -> if (v >= 0.5) day(days, d).put("kcal_active", v.roundToInt()) } }
        metric("total calories") { merged(TotalCaloriesBurnedRecord::class.java, start, end) { it.energy.inCalories / 1000 }.days().forEach { (d, v) -> if (v >= 0.5) day(days, d).put("kcal_total", v.roundToInt()) } }
        metric("heart rate") {
            daily(HeartRateRecord.BPM_MIN, from, to).forEach { (d, v) -> day(days, d).put("hr_min", v) }
            daily(HeartRateRecord.BPM_AVG, from, to).forEach { (d, v) -> day(days, d).put("hr_avg", v) }
            daily(HeartRateRecord.BPM_MAX, from, to).forEach { (d, v) -> day(days, d).put("hr_max", v) }
        }
        metric("water") { daily(HydrationRecord.VOLUME_TOTAL, from, to).forEach { (d, v) -> if (v.inLiters > 0) day(days, d).put("water_l", r1(v.inLiters)) } }
        metric("nutrition") {
            daily(NutritionRecord.ENERGY_TOTAL, from, to).forEach { (d, v) -> if (v.inCalories > 0) day(days, d).put("kcal_eaten", (v.inCalories / 1000).roundToInt()) }
            daily(NutritionRecord.PROTEIN_TOTAL, from, to).forEach { (d, v) -> if (v.inGrams > 0) day(days, d).put("protein_g", v.inGrams.roundToInt()) }
            daily(NutritionRecord.TOTAL_CARBOHYDRATE_TOTAL, from, to).forEach { (d, v) -> if (v.inGrams > 0) day(days, d).put("carbs_g", v.inGrams.roundToInt()) }
            daily(NutritionRecord.TOTAL_FAT_TOTAL, from, to).forEach { (d, v) -> if (v.inGrams > 0) day(days, d).put("fat_g", v.inGrams.roundToInt()) }
        }
    }

    // Sleep belongs to the day you woke up. The longest session is the night;
    // shorter ones that do not overlap it are naps.
    private fun sleep(days: MutableMap<LocalDate, JSONObject>, from: LocalDate, to: LocalDate, start: Instant, end: Instant) = metric("sleep") {
        val byDay = records(SleepSessionRecord::class.java, start, end).groupBy { localDate(it.endTime) }
        for ((d, list) in byDay) {
            if (d.isBefore(from) || d.isAfter(to)) continue
            val main = list.maxByOrNull { Duration.between(it.startTime, it.endTime) } ?: continue
            var awake = 0.0; var deep = 0.0; var rem = 0.0; var light = 0.0
            for (s in main.stages) {
                val h = Duration.between(s.startTime, s.endTime).toMinutes() / 60.0
                when (s.type) {
                    SleepSessionRecord.StageType.STAGE_TYPE_SLEEPING_DEEP -> deep += h
                    SleepSessionRecord.StageType.STAGE_TYPE_SLEEPING_REM -> rem += h
                    SleepSessionRecord.StageType.STAGE_TYPE_SLEEPING_LIGHT, SleepSessionRecord.StageType.STAGE_TYPE_SLEEPING -> light += h
                    SleepSessionRecord.StageType.STAGE_TYPE_AWAKE, SleepSessionRecord.StageType.STAGE_TYPE_AWAKE_IN_BED,
                    SleepSessionRecord.StageType.STAGE_TYPE_AWAKE_OUT_OF_BED -> awake += h
                }
            }
            val inBed = Duration.between(main.startTime, main.endTime).toMinutes() / 60.0
            val o = day(days, d)
            o.put("sleep_h", r2(if (main.stages.isNotEmpty()) (inBed - awake).coerceAtLeast(0.0) else inBed))
            o.put("sleep_start", main.startTime.atZone(zone).format(hhmm))
            o.put("sleep_end", main.endTime.atZone(zone).format(hhmm))
            if (deep > 0) o.put("deep_h", r2(deep))
            if (rem > 0) o.put("rem_h", r2(rem))
            if (light > 0) o.put("light_h", r2(light))
            if (awake > 0) o.put("awake_h", r2(awake))
            val naps = list.filter { it !== main && Duration.between(it.startTime, it.endTime).toHours() < 3 &&
                (it.endTime.isBefore(main.startTime) || it.startTime.isAfter(main.endTime)) }
            val napH = naps.sumOf { Duration.between(it.startTime, it.endTime).toMinutes() } / 60.0
            if (napH > 0) o.put("naps_h", r2(napH))
        }
    }

    // Measurements: the day's latest (weight, body fat, height, blood pressure)
    // or mean (resting heart rate, HRV, SpO2, breathing, glucose).
    private fun points(days: MutableMap<LocalDate, JSONObject>, from: LocalDate, to: LocalDate, start: Instant, end: Instant) {
        fun <R : Record> each(list: List<R>, time: (R) -> Instant, put: (LocalDate, List<R>) -> Unit) {
            list.groupBy { localDate(time(it)) }.forEach { (d, rs) -> if (!d.isBefore(from) && !d.isAfter(to)) put(d, rs.sortedBy(time)) }
        }
        metric("resting heart rate") { each(records(RestingHeartRateRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("hr_rest", rs.last().beatsPerMinute) } }
        metric("HRV") { each(records(HeartRateVariabilityRmssdRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("hrv", rs.map { it.heartRateVariabilityMillis }.average().roundToInt()) } }
        metric("SpO2") { each(records(OxygenSaturationRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("spo2", rs.map { it.percentage.value }.average().roundToInt()) } }
        metric("breathing") { each(records(RespiratoryRateRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("resp_rate", r1(rs.map { it.rate }.average())) } }
        metric("weight") { each(records(WeightRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("weight", r1(rs.last().weight.inGrams / 1000)) } }
        metric("body fat") { each(records(BodyFatRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("body_fat", r1(rs.last().percentage.value)) } }
        metric("height") { each(records(HeightRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("height", (rs.last().height.inMeters * 100).roundToInt()) } }
        metric("blood pressure") {
            each(records(BloodPressureRecord::class.java, start, end), { it.time }) { d, rs ->
                day(days, d).put("bp_sys", rs.last().systolic.inMillimetersOfMercury.roundToInt()).put("bp_dia", rs.last().diastolic.inMillimetersOfMercury.roundToInt())
            }
        }
        metric("glucose") { each(records(BloodGlucoseRecord::class.java, start, end), { it.time }) { d, rs -> day(days, d).put("glucose", r1(rs.map { it.level.inMillimolesPerLiter }.average())) } }
    }

    // "Running 52 min 8.1 km; Walking 20 min". The same run from the band and
    // from Strava (overlapping by half or more) is counted once.
    private fun workouts(days: MutableMap<LocalDate, JSONObject>, from: LocalDate, to: LocalDate) = metric("workouts") {
        val all = records(ExerciseSessionRecord::class.java, from.atStartOfDay(zone).toInstant(), to.plusDays(1).atStartOfDay(zone).toInstant())
            .sortedBy { it.startTime }
        val kept = ArrayList<ExerciseSessionRecord>()
        for (s in all) {
            val dup = kept.any { k ->
                val overlap = Duration.between(maxOf(k.startTime, s.startTime), minOf(k.endTime, s.endTime))
                val shorter = minOf(Duration.between(k.startTime, k.endTime), Duration.between(s.startTime, s.endTime))
                !overlap.isNegative && !shorter.isZero && overlap.toMillis() * 2 >= shorter.toMillis()
            }
            if (!dup) kept += s
        }
        for ((d, list) in kept.groupBy { localDate(it.startTime) }) {
            val parts = list.map { s ->
                val mins = Duration.between(s.startTime, s.endTime).toMinutes()
                var text = "${typeName(s.exerciseType)} $mins min"
                try {
                    total(DistanceRecord.DISTANCE_TOTAL, s.startTime, s.endTime)?.let { if (it.inMeters >= 100) text += " %.1f km".format(it.inMeters / 1000) }
                } catch (_: HealthConnectException) { } catch (_: SecurityException) { }
                text
            }
            val mins = list.sumOf { Duration.between(it.startTime, it.endTime).toMinutes() }
            day(days, d).put("workouts", list.size).put("exercise", parts.joinToString("; ")).put("active_min", mins)
        }
    }

    companion object {
        val APPS = mapOf(
            "com.sec.android.app.shealth" to "Samsung Health", "com.strava" to "Strava",
            "com.google.android.apps.fitness" to "Google Fit", "com.google.android.apps.healthdata" to "Health Connect",
            "com.fitbit.FitbitMobile" to "Fitbit", "com.xiaomi.wearable" to "Mi Fitness", "com.huawei.health" to "Huawei Health",
        )

        fun typeName(t: Int): String = when (t) {
            ExerciseSessionType.EXERCISE_SESSION_TYPE_RUNNING, ExerciseSessionType.EXERCISE_SESSION_TYPE_RUNNING_TREADMILL -> "Running"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_WALKING -> "Walking"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_HIKING -> "Hiking"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_BIKING, ExerciseSessionType.EXERCISE_SESSION_TYPE_BIKING_STATIONARY -> "Cycling"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_STRENGTH_TRAINING, ExerciseSessionType.EXERCISE_SESSION_TYPE_WEIGHTLIFTING -> "Strength training"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_CALISTHENICS -> "Calisthenics"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING -> "HIIT"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_YOGA -> "Yoga"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_PILATES -> "Pilates"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_STRETCHING -> "Stretching"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_SWIMMING_POOL, ExerciseSessionType.EXERCISE_SESSION_TYPE_SWIMMING_OPEN_WATER -> "Swimming"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_ELLIPTICAL -> "Elliptical"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_ROWING_MACHINE -> "Rowing"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_STAIR_CLIMBING, ExerciseSessionType.EXERCISE_SESSION_TYPE_STAIR_CLIMBING_MACHINE -> "Stairs"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_DANCING -> "Dancing"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_BADMINTON -> "Badminton"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_CRICKET -> "Cricket"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_TENNIS, ExerciseSessionType.EXERCISE_SESSION_TYPE_TABLE_TENNIS -> "Tennis"
            ExerciseSessionType.EXERCISE_SESSION_TYPE_BOOT_CAMP, ExerciseSessionType.EXERCISE_SESSION_TYPE_EXERCISE_CLASS,
            ExerciseSessionType.EXERCISE_SESSION_TYPE_GYMNASTICS -> "Workout class"
            else -> "Workout"
        }
    }
}
