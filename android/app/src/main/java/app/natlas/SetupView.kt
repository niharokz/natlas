package app.natlas

import android.annotation.SuppressLint
import android.app.Activity
import android.view.Gravity
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

// First run: allow file access, pick the Syncthing folder (…/notes/data), or try the sample data.
@SuppressLint("ViewConstructor")
class SetupView(
    private val act: Activity,
    private val onAccess: () -> Unit,
    private val onPick: () -> Unit,
    private val onDemo: () -> Unit,
    private val onStart: () -> Unit,
) : FrameLayout(act) {

    private val accessLine: TextView
    private val folderLine: TextView
    private val startBtn: TextView

    init {
        setBackgroundColor(Ui.color(act, R.color.bg))
        isClickable = true
        val col = Ui.column(act)
        val pad = Ui.dp(act, 28f)
        col.setPadding(pad, pad * 2, pad, pad)
        val logo = Ui.logo(act, 72f)
        col.addView(logo)
        col.addView(Ui.text(act, "natlas", 26f, R.color.fg, bold = true, mono = true), Ui.lp(act, 12f))
        col.addView(Ui.text(act, "Your lists, on the notes folder Syncthing keeps in sync with your server. Works offline; nothing leaves the phone.", 15f, R.color.muted), Ui.lp(act, 10f))

        col.addView(step("1", "Allow file access", "Natlas reads and writes the files in your synced folder."), Ui.lp(act, 30f, true))
        accessLine = Ui.text(act, "", 14f, R.color.muted)
        col.addView(accessLine, Ui.lp(act, 4f))
        col.addView(Ui.button(act, "Allow file access", primary = false) { onAccess() }, Ui.lp(act, 10f, true))

        col.addView(step("2", "Choose the folder", "The one with event.md, calendar.md … - usually …/notes/data."), Ui.lp(act, 26f, true))
        folderLine = Ui.text(act, "", 14f, R.color.muted)
        col.addView(folderLine, Ui.lp(act, 4f))
        col.addView(Ui.button(act, "Choose folder", primary = false) { onPick() }, Ui.lp(act, 10f, true))

        startBtn = Ui.button(act, "Open Natlas", primary = true) { onStart() }
        col.addView(startBtn, Ui.lp(act, 30f, true))
        col.addView(Ui.button(act, "Try it with sample data", primary = false) { onDemo() }, Ui.lp(act, 12f, true))

        val scroll = ScrollView(act).apply { isFillViewport = true; addView(col) }
        addView(scroll, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        if (Prefs(act).animations) {
            Ui.popIn(logo)
            col.alpha = 0f; col.translationY = Ui.dp(act, 16f).toFloat()
            col.animate().alpha(1f).translationY(0f).setDuration(380).start()
        }
    }

    private fun step(n: String, title: String, sub: String) = LinearLayout(act).apply {
        orientation = LinearLayout.VERTICAL
        gravity = Gravity.START
        addView(Ui.text(act, "$n  ·  $title", 17f, R.color.fg, bold = true).apply { gravity = Gravity.START })
        addView(Ui.text(act, sub, 14f, R.color.muted).apply { gravity = Gravity.START }, Ui.lp(act, 4f))
    }

    // Shows the current state; "Open Natlas" works once the folder looks right.
    fun refresh(hasAccess: Boolean, folder: String, folderOk: Boolean, note: String, canStart: Boolean) {
        accessLine.text = if (hasAccess) "✓ Allowed" else "Not allowed yet"
        accessLine.setTextColor(Ui.color(act, if (hasAccess) R.color.ok else R.color.muted))
        folderLine.text = if (folder.isEmpty()) "No folder chosen" else "$folder\n$note"
        folderLine.setTextColor(Ui.color(act, if (folderOk) R.color.ok else R.color.muted))
        startBtn.isEnabled = canStart
        startBtn.alpha = if (startBtn.isEnabled) 1f else 0.45f
    }
}
