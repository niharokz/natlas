package app.natlas

import app.natlas.health.MinuteMerge
import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.LocalDate
import java.time.ZoneId
import java.time.ZonedDateTime

class MinuteMergeTest {
    private val zone = ZoneId.of("Asia/Kolkata")
    private fun at(h: Int, m: Int, day: Int = 19) = ZonedDateTime.of(2026, 10, day, h, m, 0, 0, zone).toInstant()
    private val d19 = LocalDate.of(2026, 10, 19)

    @Test fun phoneAndBandSameWalkCountOnce() {
        val mm = MinuteMerge(zone)
        mm.add(MinuteMerge.Part("Samsung Health · phone", at(8, 0), at(8, 10), 1000.0))
        mm.add(MinuteMerge.Part("Samsung Health · band", at(8, 0), at(8, 10), 1100.0))
        mm.add(MinuteMerge.Part("Google Fit · phone", at(8, 0), at(8, 10), 950.0))
        assertEquals(1100.0, mm.days()[d19]!!, 0.01) // not 3,050
    }

    @Test fun separateWalksAdd() {
        val mm = MinuteMerge(zone)
        mm.add(MinuteMerge.Part("phone", at(8, 0), at(8, 10), 1000.0))
        mm.add(MinuteMerge.Part("band", at(18, 0), at(18, 30), 3000.0))
        assertEquals(4000.0, mm.days()[d19]!!, 0.01)
    }

    @Test fun overlapTakesBestPerMinute() {
        val mm = MinuteMerge(zone)
        mm.add(MinuteMerge.Part("phone", at(8, 0), at(8, 10), 1000.0)) // 100/min
        mm.add(MinuteMerge.Part("band", at(8, 5), at(8, 15), 1500.0))  // 150/min
        // 8:00-8:05 phone 500, 8:05-8:15 band 1500
        assertEquals(2000.0, mm.days()[d19]!!, 0.01)
    }

    @Test fun recordOverMidnightSplitsByDay() {
        val mm = MinuteMerge(zone)
        mm.add(MinuteMerge.Part("band", at(23, 50), at(0, 10, day = 20), 200.0))
        assertEquals(100.0, mm.days()[d19]!!, 0.01)
        assertEquals(100.0, mm.days()[d19.plusDays(1)]!!, 0.01)
    }

    @Test fun sourcesBreakdown() {
        val mm = MinuteMerge(zone)
        mm.add(MinuteMerge.Part("phone", at(8, 0), at(8, 10), 1000.0))
        mm.add(MinuteMerge.Part("band", at(8, 0), at(8, 10), 1100.0))
        assertEquals(listOf("band" to 1100.0, "phone" to 1000.0), mm.sources(d19))
    }
}
