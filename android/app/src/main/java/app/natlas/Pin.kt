package app.natlas

import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.PBEKeySpec

// PIN hashing: PBKDF2-SHA256, stored as "pbkdf2:<iterations>:<salt>:<key>" (base64).
object Pin {
    private const val ITER = 120_000

    fun valid(pin: String) = pin.length in 4..8 && pin.all { it in '0'..'9' }

    fun hash(pin: String, salt: ByteArray = ByteArray(16).also { SecureRandom().nextBytes(it) }, iter: Int = ITER): String {
        val key = derive(pin, salt, iter)
        val b64 = Base64.getEncoder()
        return "pbkdf2:$iter:${b64.encodeToString(salt)}:${b64.encodeToString(key)}"
    }

    fun check(pin: String, stored: String): Boolean {
        val parts = stored.split(":")
        if (parts.size != 4 || parts[0] != "pbkdf2") return false
        return try {
            val b64 = Base64.getDecoder()
            val key = derive(pin, b64.decode(parts[2]), parts[1].toInt())
            MessageDigest.isEqual(key, b64.decode(parts[3])) // constant time
        } catch (_: Exception) { false }
    }

    private fun derive(pin: String, salt: ByteArray, iter: Int): ByteArray =
        SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256").generateSecret(PBEKeySpec(pin.toCharArray(), salt, iter, 256)).encoded
}
