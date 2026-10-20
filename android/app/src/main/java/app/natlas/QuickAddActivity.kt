package app.natlas

import android.app.Activity
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.text.InputType
import android.view.Gravity
import android.view.KeyEvent
import android.view.WindowManager
import android.view.inputmethod.EditorInfo
import android.widget.EditText
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import app.natlas.widget.Snap
import org.json.JSONObject

// One-line add from a widget, without opening the app: "Call bank tomorrow #todo !high",
// "Dinner fri 8pm". The same smart parsing as the app's quick add.
class QuickAddActivity : Activity() {
    companion object { const val EXTRA_SOURCE = "source" } // "plugin/coll"

    private lateinit var input: EditText
    private lateinit var error: TextView
    private lateinit var chips: LinearLayout
    private var source = ""
    private var options: List<Pair<String, String>> = emptyList()
    private var busy = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_STATE_ALWAYS_VISIBLE)
        val snap = Snap.load(this)
        options = Snap.lists(snap, quickOnly = true)
        source = intent.getStringExtra(EXTRA_SOURCE)?.takeIf { s -> options.any { it.first == s } } ?: options.firstOrNull()?.first ?: ""

        val col = Ui.column(this).apply { gravity = Gravity.START }
        val pad = Ui.dp(this, 20f)
        col.setPadding(pad, pad, pad, Ui.dp(this, 12f))
        col.background = GradientDrawable().apply { setColor(Ui.color(this@QuickAddActivity, R.color.surface)); cornerRadius = Ui.dp(this@QuickAddActivity, 24f).toFloat() }
        col.addView(Ui.text(this, "Quick add", 18f, R.color.fg, bold = true).apply { gravity = Gravity.START })

        chips = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
        col.addView(HorizontalScrollView(this).apply { isHorizontalScrollBarEnabled = false; addView(chips) }, Ui.lp(this, 10f, true))

        input = EditText(this).apply {
            setSingleLine()
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_FLAG_CAP_SENTENCES
            imeOptions = EditorInfo.IME_ACTION_DONE
            textSize = 17f
            setTextColor(Ui.color(this@QuickAddActivity, R.color.fg))
            setHintTextColor(Ui.color(this@QuickAddActivity, R.color.muted))
            setOnEditorActionListener { _, action, ev ->
                if (action == EditorInfo.IME_ACTION_DONE || ev?.keyCode == KeyEvent.KEYCODE_ENTER) { add(); true } else false
            }
        }
        col.addView(input, Ui.lp(this, 10f, true))
        error = Ui.text(this, "", 13f, R.color.bad).apply { gravity = Gravity.START }
        col.addView(error, Ui.lp(this, 2f, true))

        val row = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL; gravity = Gravity.END }
        row.addView(Ui.button(this, "Open app", primary = false) {
            startActivity(android.content.Intent(this, MainActivity::class.java).putExtra(MainActivity.EXTRA_ROUTE, "#/p/" + source.substringBefore("/") + "/" + source.substringAfter("/") + "/new")
                .addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK))
            finish()
        })
        row.addView(Ui.button(this, "Add", primary = true) { add() }, LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply { marginStart = Ui.dp(this@QuickAddActivity, 10f) })
        col.addView(row, Ui.lp(this, 14f, true))

        setContentView(col)
        window.setLayout(resources.displayMetrics.widthPixels * 92 / 100, WindowManager.LayoutParams.WRAP_CONTENT)
        drawChips(snap)
        if (options.isEmpty()) { error.text = "Open Natlas once so the lists are known."; input.isEnabled = false }
        input.requestFocus()
        if (Prefs(this).animations) Ui.popIn(col)
    }

    private fun drawChips(snap: JSONObject?) {
        chips.removeAllViews()
        for ((key, label) in options) {
            val on = key == source
            val chip = TextView(this).apply {
                text = label
                textSize = 13f
                val p = Ui.dp(this@QuickAddActivity, 12f)
                setPadding(p, p / 2, p, p / 2)
                setTextColor(Ui.color(this@QuickAddActivity, if (on) R.color.on_accent else R.color.fg))
                background = GradientDrawable().apply {
                    cornerRadius = 999f
                    if (on) setColor(Ui.color(this@QuickAddActivity, R.color.accent)) else setStroke(Ui.dp(this@QuickAddActivity, 1f), Ui.color(this@QuickAddActivity, R.color.line))
                }
                setOnClickListener { source = key; drawChips(snap) }
            }
            chips.addView(chip, LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply { marginEnd = Ui.dp(this@QuickAddActivity, 8f) })
        }
        input.hint = hintFor(snap, source)
    }

    private fun hintFor(snap: JSONObject?, key: String): String {
        if (snap == null) return "What to add?"
        for (p in Snap.objs(snap.optJSONArray("plugins"))) for (c in Snap.objs(p.optJSONArray("collections")))
            if ("${p.optString("id")}/${c.optString("id")}" == key) return c.optString("hint").ifBlank { "Title" }
        return "Title"
    }

    private fun add() {
        val text = input.text.toString().trim()
        if (text.isEmpty() || busy || source.isEmpty()) return
        busy = true
        error.text = ""
        val app = applicationContext
        val req = JSONObject().put("op", "quick").put("plugin", source.substringBefore("/")).put("coll", source.substringAfter("/")).put("text", text)
        Thread {
            val res = Engine.cli(app, req)
            runOnUiThread {
                busy = false
                if (res.optInt("status") == 201) {
                    val rec = res.optJSONObject("record") ?: JSONObject()
                    val date = rec.optString("date").takeIf { it.isNotEmpty() && it != "null" }?.let { " · $it" } ?: ""
                    Toast.makeText(app, "Added: ${rec.optString("title", text)}$date", Toast.LENGTH_SHORT).show()
                    Thread { Refresh.now(app) }.start()
                    finish()
                } else {
                    error.text = res.optString("error", "Could not add").take(160)
                }
            }
        }.start()
    }
}
