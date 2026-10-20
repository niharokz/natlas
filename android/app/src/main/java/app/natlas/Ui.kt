package app.natlas

import android.animation.ObjectAnimator
import android.content.Context
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.graphics.drawable.RippleDrawable
import android.content.res.ColorStateList
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.animation.OvershootInterpolator
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView

// Small helpers for the native screens (splash, lock, setup), drawn in code so
// they match the web app's nss look without any layout library.
object Ui {
    fun dp(ctx: Context, v: Float): Int =
        TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, ctx.resources.displayMetrics).toInt()

    fun color(ctx: Context, id: Int) = ctx.getColor(id)

    fun text(ctx: Context, s: String, sizeSp: Float, colorId: Int, bold: Boolean = false, mono: Boolean = false) =
        TextView(ctx).apply {
            text = s
            textSize = sizeSp
            setTextColor(color(ctx, colorId))
            gravity = Gravity.CENTER
            typeface = Typeface.create(if (mono) Typeface.MONOSPACE else Typeface.DEFAULT, if (bold) Typeface.BOLD else Typeface.NORMAL)
        }

    // Rounded button: filled (primary) or outlined.
    fun button(ctx: Context, label: String, primary: Boolean, onClick: () -> Unit) = TextView(ctx).apply {
        text = label
        textSize = 16f
        gravity = Gravity.CENTER
        typeface = Typeface.create(Typeface.DEFAULT, Typeface.BOLD)
        val pad = dp(ctx, 14f)
        setPadding(pad * 2, pad, pad * 2, pad)
        val bg = GradientDrawable().apply {
            cornerRadius = dp(ctx, 14f).toFloat()
            if (primary) setColor(color(ctx, R.color.accent)) else {
                setColor(color(ctx, R.color.surface)); setStroke(dp(ctx, 1f), color(ctx, R.color.line))
            }
        }
        background = RippleDrawable(ColorStateList.valueOf(color(ctx, R.color.ripple)), bg, null)
        setTextColor(color(ctx, if (primary) R.color.on_accent else R.color.fg))
        isClickable = true
        setOnClickListener { onClick() }
    }

    fun column(ctx: Context) = LinearLayout(ctx).apply {
        orientation = LinearLayout.VERTICAL
        gravity = Gravity.CENTER_HORIZONTAL
    }

    fun logo(ctx: Context, sizeDp: Float) = ImageView(ctx).apply {
        setImageResource(R.drawable.logo)
        layoutParams = LinearLayout.LayoutParams(dp(ctx, sizeDp), dp(ctx, sizeDp))
    }

    fun lp(ctx: Context, topDp: Float = 0f, matchWidth: Boolean = false) = LinearLayout.LayoutParams(
        if (matchWidth) LinearLayout.LayoutParams.MATCH_PARENT else LinearLayout.LayoutParams.WRAP_CONTENT,
        LinearLayout.LayoutParams.WRAP_CONTENT,
    ).apply { topMargin = dp(ctx, topDp) }

    // Pop in: fade + slight scale with a soft overshoot.
    fun popIn(v: View, delay: Long = 0, animate: Boolean = true) {
        if (!animate) return
        v.alpha = 0f; v.scaleX = 0.85f; v.scaleY = 0.85f
        v.animate().alpha(1f).scaleX(1f).scaleY(1f).setStartDelay(delay).setDuration(420)
            .setInterpolator(OvershootInterpolator(1.4f)).start()
    }

    // Wrong PIN: shake sideways.
    fun shake(v: View) {
        ObjectAnimator.ofFloat(v, "translationX", 0f, 24f, -24f, 16f, -16f, 8f, -8f, 0f).setDuration(420).start()
    }
}
