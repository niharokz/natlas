package app.natlas

import android.app.Activity
import android.app.KeyguardManager
import android.content.Context
import android.hardware.biometrics.BiometricManager
import android.hardware.biometrics.BiometricPrompt
import android.os.CancellationSignal
import android.os.SystemClock

// When the app must be unlocked, and the system fingerprint / screen-lock prompt.
object Lock {
    @Volatile private var unlockedAt = 0L      // elapsed time of the last unlock (0 = locked)
    @Volatile private var backgroundAt = 0L    // elapsed time the app left the screen

    fun needed(prefs: Prefs): Boolean {
        if (prefs.lockMode == "off") return false
        if (prefs.lockMode == "pin" && prefs.pinHash.isEmpty()) return false
        if (unlockedAt == 0L) return true
        val away = if (backgroundAt == 0L) 0L else (SystemClock.elapsedRealtime() - backgroundAt) / 1000
        return when (val d = prefs.lockDelay) {
            -1 -> false
            0 -> backgroundAt != 0L
            else -> away >= d
        }
    }

    fun unlocked() { unlockedAt = SystemClock.elapsedRealtime(); backgroundAt = 0L }
    fun lockNow() { unlockedAt = 0L }
    fun leftApp() { if (unlockedAt != 0L) backgroundAt = SystemClock.elapsedRealtime() }
    fun cameBack() { /* the delay is checked in needed(); nothing to reset here */ }

    fun deviceSecure(ctx: Context) = ctx.getSystemService(KeyguardManager::class.java)?.isDeviceSecure == true

    fun biometricReady(ctx: Context): Boolean =
        ctx.getSystemService(BiometricManager::class.java)
            ?.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_WEAK) == BiometricManager.BIOMETRIC_SUCCESS

    // device: fingerprint/face with the phone's screen lock as backup.
    // fingerprintOnly: fingerprint/face, with a "Use PIN" button (Natlas PIN mode).
    fun prompt(activity: Activity, fingerprintOnly: Boolean, onOk: () -> Unit, onCancel: () -> Unit) {
        val b = BiometricPrompt.Builder(activity)
            .setTitle("Unlock Natlas")
            .setConfirmationRequired(false)
        if (fingerprintOnly) {
            b.setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_WEAK)
            b.setNegativeButton("Use PIN", activity.mainExecutor) { _, _ -> onCancel() }
        } else {
            b.setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
        }
        try {
            b.build().authenticate(CancellationSignal(), activity.mainExecutor, object : BiometricPrompt.AuthenticationCallback() {
                override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) = onOk()
                override fun onAuthenticationError(errorCode: Int, errString: CharSequence) = onCancel()
            })
        } catch (_: Exception) {
            onCancel()
        }
    }
}
