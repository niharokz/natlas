package app.natlas

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PathsTest {
    @Test fun primary() {
        assertEquals("/storage/emulated/0/Sync/notes/data", Paths.fromTreeId("primary:Sync/notes/data"))
        assertEquals("/storage/emulated/0", Paths.fromTreeId("primary:"))
    }

    @Test fun sdCard() {
        assertEquals("/storage/1A2B-3C4D/notes", Paths.fromTreeId("1A2B-3C4D:notes/"))
    }

    @Test fun noVolume() {
        assertNull(Paths.fromTreeId("notes"))
    }

    @Test fun describe() {
        assertFalse(Paths.describe(emptyList()).first)
        assertFalse(Paths.describe(listOf("todo.txt")).first)
        val (ok, note) = Paths.describe(listOf("event.md", "calendar.md", "x.txt"))
        assertTrue(ok)
        assertEquals("Found event.md, calendar.md", note)
    }
}
