package app.natlas

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PinTest {
    @Test fun validLength() {
        assertTrue(Pin.valid("1234"))
        assertTrue(Pin.valid("12345678"))
        assertFalse(Pin.valid("123"))
        assertFalse(Pin.valid("123456789"))
        assertFalse(Pin.valid("12a4"))
    }

    @Test fun hashAndCheck() {
        val h = Pin.hash("4821", iter = 1000) // low count keeps the test fast
        assertEquals(4, h.split(":").size)
        assertTrue(Pin.check("4821", h))
        assertFalse(Pin.check("4822", h))
    }

    @Test fun saltDiffers() {
        assertFalse(Pin.hash("4821", iter = 1000) == Pin.hash("4821", iter = 1000))
    }

    @Test fun badStored() {
        assertFalse(Pin.check("4821", ""))
        assertFalse(Pin.check("4821", "plain:4821"))
        assertFalse(Pin.check("4821", "pbkdf2:x:y:z"))
    }
}
