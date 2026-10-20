package app.natlas.widget

import android.content.Context
import android.graphics.Color
import android.widget.RemoteViews
import app.natlas.Prefs

// Widget colours. Each colour is a (light, dark) pair: with "Follow the phone"
// the launcher switches by itself when the phone changes theme.
class Look(ctx: Context) {
    private val prefs = Prefs(ctx)
    val theme = prefs.widgetTheme
    val privacy = prefs.widgetPrivacy
    private val alpha = (prefs.widgetOpacity * 255 / 100).coerceIn(0, 255)

    private fun pair(light: String, dark: String): Pair<Int, Int> {
        val l = Color.parseColor(light); val d = Color.parseColor(dark)
        return when (theme) {
            "light" -> l to l
            "dark", "glass", "clear" -> d to d
            "amoled" -> d to d
            else -> l to d
        }
    }

    val bg: Pair<Int, Int> = when (theme) {
        "amoled" -> Color.BLACK to Color.BLACK
        "glass" -> Color.BLACK to Color.BLACK
        else -> pair("#FFFFFF", "#0F161B")
    }
    val bgAlpha: Int = when (theme) { "clear" -> 0; "glass" -> 90; else -> alpha }
    val text = pair("#14201F", "#E6ECEC")
    val muted = pair("#5C6B6A", "#9AA6A8")
    val line = pair("#D7E2E0", "#24323A")
    val accent: Pair<Int, Int> = if (prefs.widgetAccent == "dynamic")
        ctx.getColor(android.R.color.system_accent1_600) to ctx.getColor(android.R.color.system_accent1_200)
    else pair("#2C6A64", "#66BDB3")
    val onAccent = pair("#FFFFFF", "#0A1412")
    val chip = pair("#EAF1F0", "#152027")

    fun tone(t: String): Pair<Int, Int> = when (t) {
        "warn" -> pair("#A86A00", "#E0A63A")
        "bad" -> pair("#D63C3C", "#FF6B6B")
        "ok" -> pair("#1F8A5A", "#3FB27F")
        "accent" -> accent
        else -> muted
    }

    // Calendar category colours (same as the app).
    fun category(cat: String): Int = Color.parseColor(when (cat) {
        "birthday" -> "#E67C73"; "anniversary" -> "#D81B60"; "holiday" -> "#0B8043"; "festival" -> "#F09300"
        "reservation" -> "#8E24AA"; "reminder" -> "#F4511E"; "out_of_office" -> "#7986CB"; "focus_time" -> "#3F51B5"
        "working_location" -> "#33B679"; else -> "#4285F4"
    })

    fun text(rv: RemoteViews, id: Int, c: Pair<Int, Int>) = rv.setColorInt(id, "setTextColor", c.first, c.second)

    fun background(rv: RemoteViews, id: Int) {
        rv.setColorInt(id, "setColorFilter", bg.first, bg.second)
        rv.setInt(id, "setImageAlpha", bgAlpha)
    }

    fun mask(s: String) = if (privacy) "• • •" else s
}
