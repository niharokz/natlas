package app.natlas

import android.annotation.SuppressLint
import android.app.Activity
import android.graphics.drawable.GradientDrawable
import android.view.Gravity
import android.view.HapticFeedbackConstants
import android.view.View
import android.widget.FrameLayout
import android.widget.GridLayout
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView

// The lock screen (and the "choose a PIN" screen), drawn natively over the
// WebView so nothing of the app shows until it is unlocked.
@SuppressLint("ViewConstructor")
class LockView(
    private val act: Activity,
    private val mode: Mode,
    private val onDone: (pinHash: String?, pinLength: Int) -> Unit, // unlocked, or the new PIN
    private val onCancel: () -> Unit = {},
) : FrameLayout(act) {

    enum class Mode { UNLOCK, SET_PIN }

    private val prefs = Prefs(act)
    private val anim = prefs.animations
    private val subtitle: TextView
    private val dots = LinearLayout(act).apply { orientation = LinearLayout.HORIZONTAL; gravity = Gravity.CENTER }
    private var typed = StringBuilder()
    private var firstPin = "" // set mode: the PIN typed the first time

    init {
        setBackgroundColor(Ui.color(act, R.color.bg))
        isClickable = true // swallow touches meant for the app underneath
        val col = Ui.column(act)
        val logo = Ui.logo(act, 84f)
        col.addView(logo)
        col.addView(Ui.text(act, "natlas", 26f, R.color.fg, bold = true, mono = true), Ui.lp(act, 14f))
        subtitle = Ui.text(act, "", 15f, R.color.muted)
        col.addView(subtitle, Ui.lp(act, 6f))
        col.addView(dots, Ui.lp(act, 26f))
        val usePad = mode == Mode.SET_PIN || prefs.lockMode == "pin"
        if (usePad) col.addView(keypad(), Ui.lp(act, 26f)) else {
            val btn = Ui.button(act, "Unlock", primary = true) { askBiometric() }
            btn.setCompoundDrawablesRelativeWithIntrinsicBounds(R.drawable.ic_fingerprint, 0, 0, 0)
            btn.compoundDrawablePadding = Ui.dp(act, 10f)
            col.addView(btn, Ui.lp(act, 40f))
        }
        if (mode == Mode.SET_PIN) {
            col.addView(Ui.button(act, "Cancel", primary = false) { onCancel() }, Ui.lp(act, 18f))
        }
        addView(col, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        Ui.popIn(logo, 0, anim)
        Ui.popIn(dots, 120, anim)
        reset(if (mode == Mode.SET_PIN) "Choose a PIN (4 to 8 digits)" else if (usePad) "Enter your PIN" else "Locked")
    }

    // Call once the view is on screen: fingerprint first where it applies.
    fun start() {
        if (mode != Mode.UNLOCK) return
        if (prefs.lockMode == "device") askBiometric()
        else if (prefs.pinBio && Lock.biometricReady(act)) askBiometric()
    }

    private fun askBiometric() {
        val pinMode = prefs.lockMode == "pin"
        Lock.prompt(act, fingerprintOnly = pinMode, onOk = { unlock() }, onCancel = {})
    }

    private fun unlock() {
        Lock.unlocked()
        prefs.pinFails = 0
        if (!anim) { onDone(null, 0); return }
        animate().alpha(0f).scaleX(1.06f).scaleY(1.06f).setDuration(260).withEndAction { onDone(null, 0) }.start()
    }

    private fun keypad(): View {
        val grid = GridLayout(act).apply { columnCount = 3 }
        val keys = listOf("1", "2", "3", "4", "5", "6", "7", "8", "9", "bio", "0", "del")
        for (k in keys) {
            val size = Ui.dp(act, 76f)
            val v: View = when (k) {
                "bio" -> if (mode == Mode.UNLOCK && prefs.pinBio && Lock.biometricReady(act))
                    ImageView(act).apply { setImageResource(R.drawable.ic_fingerprint); scaleType = ImageView.ScaleType.CENTER; setOnClickListener { askBiometric() } }
                else if (mode == Mode.SET_PIN) Ui.text(act, "OK", 18f, R.color.accent, bold = true).apply { setOnClickListener { submit() } }
                else View(act)
                "del" -> ImageView(act).apply { setImageResource(R.drawable.ic_backspace); scaleType = ImageView.ScaleType.CENTER; setOnClickListener { erase() } }
                else -> Ui.text(act, k, 28f, R.color.fg).apply { setOnClickListener { press(k) } }
            }
            if (k != "bio" || v.hasOnClickListeners()) {
                v.background = act.getDrawable(R.drawable.key_ripple)
            }
            grid.addView(v, GridLayout.LayoutParams().apply {
                width = size; height = size
                val m = Ui.dp(act, 6f); setMargins(m, m, m, m)
            })
        }
        return grid
    }

    private fun press(d: String) {
        if (typed.length >= 8) return
        val paused = prefs.pinPausedUntil - System.currentTimeMillis()
        if (mode == Mode.UNLOCK && paused > 0) { subtitle.text = "Too many tries - wait ${paused / 1000 + 1} s"; return }
        typed.append(d)
        performHapticFeedback(HapticFeedbackConstants.KEYBOARD_TAP)
        drawDots()
        if (mode == Mode.UNLOCK && typed.length == prefs.pinLength) submit()
    }

    private fun erase() {
        if (typed.isNotEmpty()) typed.setLength(typed.length - 1)
        drawDots()
    }

    private fun submit() {
        val pin = typed.toString()
        when (mode) {
            Mode.UNLOCK -> if (Pin.check(pin, prefs.pinHash)) unlock() else {
                prefs.pinFails = prefs.pinFails + 1
                if (prefs.pinFails >= 5) { prefs.pinPausedUntil = System.currentTimeMillis() + 30_000; prefs.pinFails = 0 }
                wrong("Wrong PIN")
            }
            Mode.SET_PIN -> when {
                !Pin.valid(pin) -> wrong("Use 4 to 8 digits")
                firstPin.isEmpty() -> { firstPin = pin; reset("Enter the same PIN again") }
                firstPin != pin -> { firstPin = ""; wrong("The PINs differ - choose again") }
                else -> onDone(Pin.hash(pin), pin.length)
            }
        }
    }

    private fun wrong(msg: String) {
        performHapticFeedback(HapticFeedbackConstants.REJECT)
        if (anim) Ui.shake(dots)
        reset(msg)
    }

    private fun reset(msg: String) {
        typed.setLength(0)
        subtitle.text = msg
        drawDots()
    }

    // PIN dots: as many as the PIN when unlocking, as typed when choosing.
    private fun drawDots() {
        dots.removeAllViews()
        val count = if (mode == Mode.UNLOCK && prefs.lockMode == "pin") prefs.pinLength.coerceAtLeast(4) else maxOf(4, typed.length)
        if (mode == Mode.UNLOCK && prefs.lockMode != "pin") return
        for (i in 0 until count) {
            val filled = i < typed.length
            val d = View(act).apply {
                background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    if (filled) setColor(Ui.color(act, R.color.accent)) else setStroke(Ui.dp(act, 2f), Ui.color(act, R.color.muted))
                }
            }
            val s = Ui.dp(act, 14f)
            dots.addView(d, LinearLayout.LayoutParams(s, s).apply { val m = Ui.dp(act, 8f); setMargins(m, 0, m, 0) })
            if (filled && i == typed.length - 1 && anim) Ui.popIn(d)
        }
    }
}
