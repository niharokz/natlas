package app.natlas

import android.animation.ObjectAnimator
import android.animation.ValueAnimator
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.view.WindowInsetsController
import android.view.WindowManager
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import app.natlas.health.HealthConnect
import app.natlas.health.HealthSync
import org.json.JSONObject
import java.net.URLEncoder
import java.util.concurrent.Executors

// The screen: first-run setup, a splash while the engine starts, the WebView
// with the Natlas app, and the lock screen on top when needed.
class MainActivity : Activity() {
    companion object {
        const val EXTRA_ROUTE = "route" // e.g. "#/p/events/events/r/call-bank" from a widget
        private const val REQ_PICK = 41
        private const val REQ_HEALTH = 42
        private const val REQ_CONFIG = 43
    }

    private val prefs by lazy { Prefs(this) }
    private val bg = Executors.newSingleThreadExecutor()
    private lateinit var root: FrameLayout
    private var web: WebView? = null
    private var splash: View? = null
    private var lock: LockView? = null
    private var pinSetter: LockView? = null
    private var setup: SetupView? = null
    private var pendingRoute: String? = null
    private var pageReady = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        applySecure()
        root = FrameLayout(this).apply { setBackgroundColor(Ui.color(this@MainActivity, R.color.bg)) }
        setContentView(root)
        pendingRoute = intent.getStringExtra(EXTRA_ROUTE)
        if (intent.action == Intent.ACTION_VIEW_PERMISSION_USAGE) pendingRoute = "#/phone" // "why does Natlas read health data?"
        if (Storage.canStart(this, prefs.folder)) boot() else showSetup()
    }

    override fun onStart() {
        super.onStart()
        showLockIfNeeded()
        if (web != null && !Engine.alive()) restartEngine() // Android may have killed it while away
    }

    override fun onResume() {
        super.onResume()
        refreshSetup() // back from the "All files access" screen
        if (web != null) HealthSync.catchUp(this) // missed the morning sync (phone was off)
    }

    override fun onStop() {
        super.onStop()
        Lock.leftApp()
        if (web != null) bg.execute { Refresh.now(applicationContext) } // widgets show what you just did
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        val route = intent.getStringExtra(EXTRA_ROUTE) ?: return
        if (pageReady) go(route) else pendingRoute = route
    }

    private fun go(route: String) {
        web?.evaluateJavascript("location.hash=${JSONObject.quote(route.removePrefix("#"))}", null)
    }

    // ── engine + page ──────────────────────────────────────────────────

    private fun boot() {
        removeSetup()
        showSplash()
        bg.execute {
            val ok = Engine.start(applicationContext)
            runOnUiThread { if (ok) loadWeb() else showError(Engine.lastError) }
        }
    }

    fun restartEngine() {
        pageReady = false
        showSplash()
        bg.execute {
            val ok = Engine.restart(applicationContext)
            runOnUiThread { if (ok) loadWeb() else showError(Engine.lastError) }
        }
    }

    @SuppressLint("SetJavaScriptEnabled", "JavascriptInterface")
    private fun loadWeb() {
        val w = web ?: WebView(this).also { w ->
            w.setBackgroundColor(Ui.color(this, R.color.bg))
            w.overScrollMode = View.OVER_SCROLL_NEVER
            w.settings.apply {
                javaScriptEnabled = true
                domStorageEnabled = true
                allowFileAccess = false
                allowContentAccess = false
                setSupportZoom(false)
                textZoom = 100
            }
            w.addJavascriptInterface(Bridge(this), "NatlasNative")
            w.webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView, req: WebResourceRequest): Boolean {
                    if (req.url.host == "127.0.0.1") return false
                    try { startActivity(Intent(Intent.ACTION_VIEW, req.url)) } catch (_: Exception) { }
                    return true // outside links open in the browser, never inside the app
                }

                override fun onPageFinished(view: WebView, url: String) {
                    if (!url.contains("/app")) return
                    pageReady = true
                    hideSplash()
                    pendingRoute?.let { go(it); pendingRoute = null }
                }

                override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                    root.removeView(view); view.destroy(); web = null
                    loadWeb() // the page crashed or was killed: start a fresh one
                    return true
                }
            }
            root.addView(w, 0, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
            web = w
        }
        val next = "/app" + (pendingRoute?.takeIf { it.startsWith("#") } ?: prefs.start)
        pendingRoute = null
        w.loadUrl(Engine.url("/local?t=${Engine.token}&next=" + URLEncoder.encode(next, "UTF-8")))
    }

    fun emit(name: String, data: Any) = runOnUiThread {
        web?.evaluateJavascript("window.natlasNativeEvent&&natlasNativeEvent(${JSONObject.quote(name)},$data)", null)
    }

    // ── splash / error ─────────────────────────────────────────────────

    private fun showSplash() {
        if (splash != null) return
        val col = Ui.column(this)
        val logo = Ui.logo(this, 96f)
        col.addView(logo)
        col.addView(Ui.text(this, "natlas", 24f, R.color.fg, bold = true, mono = true), Ui.lp(this, 14f))
        val box = FrameLayout(this).apply {
            setBackgroundColor(Ui.color(this@MainActivity, R.color.bg))
            isClickable = true
            addView(col, FrameLayout.LayoutParams(FrameLayout.LayoutParams.WRAP_CONTENT, FrameLayout.LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        }
        root.addView(box, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        lock?.bringToFront()
        splash = box
        if (prefs.animations) { // a slow breathing pulse while the engine starts
            Ui.popIn(logo)
            ObjectAnimator.ofFloat(logo, "alpha", 1f, 0.55f).apply {
                duration = 900; startDelay = 450; repeatMode = ValueAnimator.REVERSE; repeatCount = ValueAnimator.INFINITE
            }.start()
        }
    }

    private fun hideSplash() {
        val s = splash ?: return
        splash = null
        if (!prefs.animations) { root.removeView(s); return }
        s.animate().alpha(0f).setDuration(220).withEndAction { root.removeView(s) }.start()
    }

    private fun showError(msg: String) {
        splash?.let { root.removeView(it) }
        splash = null
        val col = Ui.column(this)
        val pad = Ui.dp(this, 28f)
        col.setPadding(pad, pad, pad, pad)
        col.addView(Ui.logo(this, 64f))
        col.addView(Ui.text(this, "Natlas couldn't start", 20f, R.color.fg, bold = true), Ui.lp(this, 16f))
        col.addView(Ui.text(this, msg.ifBlank { "Unknown error" }, 13f, R.color.muted, mono = true), Ui.lp(this, 10f))
        val box = FrameLayout(this).apply { setBackgroundColor(Ui.color(this@MainActivity, R.color.bg)); isClickable = true }
        col.addView(Ui.button(this, "Try again", primary = true) { root.removeView(box); restartEngine() }, Ui.lp(this, 24f, true))
        col.addView(Ui.button(this, "Choose another folder", primary = false) { root.removeView(box); showSetup() }, Ui.lp(this, 10f, true))
        box.addView(col, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        root.addView(box, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
    }

    // ── first run ──────────────────────────────────────────────────────

    private fun showSetup() {
        if (setup != null) return
        val v = SetupView(this,
            onAccess = { startActivity(Storage.accessIntent(this)) },
            onPick = { pickFolder() },
            onDemo = { useDemo() },
            onStart = { boot() })
        root.addView(v, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        setup = v
        refreshSetup()
    }

    private fun refreshSetup() {
        val s = setup ?: return
        val (ok, note) = Storage.check(this, prefs.folder)
        s.refresh(Storage.hasAccess(), if (Storage.isDemo(this, prefs.folder)) "Sample data" else prefs.folder, ok, note, Storage.canStart(this, prefs.folder))
    }

    private fun removeSetup() { setup?.let { root.removeView(it) }; setup = null }

    fun pickFolder() {
        try { startActivityForResult(Storage.pickIntent(), REQ_PICK) } catch (_: Exception) { }
    }

    fun pickConfigFolder() {
        try { startActivityForResult(Storage.pickIntent(), REQ_CONFIG) } catch (_: Exception) { }
    }

    // Config folder chosen: take its settings.json if it has one, else write ours there.
    fun useConfigFolder(path: String) {
        prefs.configDir = path
        val loaded = AppConfig.load(this)
        if (!loaded) AppConfig.save(this)
        applySecure()
        HealthSync.schedule(this)
        emit("toast", JSONObject().put("text", if (loaded) "Settings loaded from the config folder" else "Settings saved to the config folder"))
        restartEngine() // natlas.yml / plugins may differ
    }

    @Deprecated("framework Activity result API (no AndroidX here)")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION") super.onActivityResult(requestCode, resultCode, data)
        if (resultCode != RESULT_OK) return
        val path = data?.data?.let { Storage.pathOf(it) } ?: return
        if (requestCode == REQ_CONFIG) { useConfigFolder(path); return }
        if (requestCode != REQ_PICK) return
        prefs.folder = path
        AppConfig.save(this)
        if (setup != null) refreshSetup() else restartEngine()
        emit("settings", Bridge(this).settings())
    }

    // Copies the built-in sample data into the app's own folder and opens it.
    fun useDemo() {
        val dir = Storage.demoDir(this)
        bg.execute {
            val res = Engine.cli(applicationContext, JSONObject().put("op", "demo").put("dir", dir.path))
            runOnUiThread {
                if (res.optInt("status") != 200) { showError(res.optString("error")); return@runOnUiThread }
                prefs.folder = dir.path
                if (setup != null) boot() else restartEngine()
            }
        }
    }

    // ── health ─────────────────────────────────────────────────────────

    // Health Connect's own permission screen (one switch per data type).
    fun requestHealth() {
        if (!HealthSync.supported()) { emit("toast", JSONObject().put("text", "Health Connect needs Android 14 or newer").put("type", "error")); return }
        requestPermissions(HealthConnect.ALL.toTypedArray(), REQ_HEALTH)
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQ_HEALTH) return
        val n = HealthConnect.grantedCount(this)
        if (n > 0 && !prefs.healthOn) { prefs.healthOn = true; HealthSync.schedule(this) }
        emit("settings", Bridge(this).settings())
        emit("toast", JSONObject().put("text", if (n > 0) "Health Connect: $n of ${HealthConnect.READ.size} data types allowed" else "No health data allowed").put("type", if (n > 0) "ok" else "error"))
    }

    // ── lock ───────────────────────────────────────────────────────────

    fun showLockIfNeeded() {
        if (lock != null || !Lock.needed(prefs)) return
        val v = LockView(this, LockView.Mode.UNLOCK, onDone = { _, _ -> lock?.let { root.removeView(it) }; lock = null })
        root.addView(v, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        lock = v
        v.post { v.start() }
    }

    // Choose a PIN; on success the lock mode becomes thenMode ("pin").
    fun choosePin(thenMode: String) {
        if (pinSetter != null) return
        val v = LockView(this, LockView.Mode.SET_PIN, onDone = { hash, len ->
            prefs.pinHash = hash ?: ""
            prefs.pinLength = len
            prefs.lockMode = if (thenMode == "off") "pin" else thenMode
            AppConfig.save(this)
            Lock.unlocked()
            pinSetter?.let { root.removeView(it) }; pinSetter = null
            emit("settings", Bridge(this).settings())
            emit("toast", JSONObject().put("text", "PIN saved"))
        }, onCancel = {
            pinSetter?.let { root.removeView(it) }; pinSetter = null
            emit("settings", Bridge(this).settings())
        })
        root.addView(v, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
        pinSetter = v
    }

    fun applySecure() {
        if (prefs.secure) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        else window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
    }

    // System bars match the page: dark icons on light, light icons on dark.
    fun setBars(dark: Boolean) {
        val c = if (dark) Color.parseColor("#070b0f") else Color.parseColor("#f6f8f8")
        @Suppress("DEPRECATION") run { window.statusBarColor = c; window.navigationBarColor = c }
        val light = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS or WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS
        window.insetsController?.setSystemBarsAppearance(if (dark) 0 else light, light)
    }

    @Deprecated("framework back handling (no AndroidX here)")
    override fun onBackPressed() {
        when {
            pinSetter != null -> { root.removeView(pinSetter); pinSetter = null }
            lock != null -> moveTaskToBack(true)
            web?.canGoBack() == true -> web?.goBack() // closes a sheet, or the previous screen
            else -> moveTaskToBack(true) // keep the engine warm instead of finishing
        }
    }

    override fun onDestroy() {
        web?.destroy()
        web = null
        super.onDestroy()
    }
}
