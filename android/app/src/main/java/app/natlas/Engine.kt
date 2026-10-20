package app.natlas

import android.content.Context
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.InetAddress
import java.net.ServerSocket
import java.net.URL
import java.security.SecureRandom
import java.util.TimeZone
import java.util.concurrent.TimeUnit

// Starts and stops the Go engine (libnatlas.so, run as a program) that serves
// the app on 127.0.0.1, and runs one-off `natlas cli` requests for widgets.
object Engine {
    @Volatile var port = 0
        private set
    @Volatile var token = ""
        private set
    @Volatile var lastError = ""
        private set
    private var proc: Process? = null

    fun binary(ctx: Context) = File(ctx.applicationInfo.nativeLibraryDir, "libnatlas.so")
    fun logFile(ctx: Context) = File(ctx.filesDir, "engine.log")

    fun url(path: String) = "http://127.0.0.1:$port$path"

    fun alive(): Boolean = proc?.isAlive == true

    private fun randomToken(): String {
        val b = ByteArray(24)
        SecureRandom().nextBytes(b)
        return b.joinToString("") { "%02x".format(it) }
    }

    // Settings the engine reads from its environment (no natlas.yml on the phone).
    fun env(ctx: Context, prefs: Prefs, tok: String): Map<String, String> {
        val locks = File(ctx.filesDir, "locks").apply { mkdirs() }
        return mapOf(
            "NATLAS_MODE" to "local",
            "NATLAS_TOKEN" to tok,
            "NATLAS_DATA_DIR" to prefs.folder,
            "NATLAS_PLUGINS" to prefs.plugins,
            "NATLAS_MAIN" to if (prefs.main) "1" else "0",
            "NATLAS_LOCK_DIR" to locks.path,
            "NATLAS_CONFIG" to AppConfig.configFile(ctx).path, // absent: built-in defaults
            "NATLAS_PLUGINS_DIR" to AppConfig.pluginsDir(ctx).path, // absent (or a plugin missing): built-in plugins
            "TZ" to TimeZone.getDefault().id,
            "HOME" to ctx.filesDir.path,
        )
    }

    private fun freePort(prefer: Int): Int {
        for (candidate in listOf(prefer) + List(20) { 20000 + SecureRandom().nextInt(40000) }) {
            if (candidate <= 0) continue
            try {
                ServerSocket(candidate, 1, InetAddress.getByName("127.0.0.1")).close()
                return candidate
            } catch (_: Exception) { }
        }
        return 0
    }

    // Starts the engine (or keeps the running one) and waits until it answers.
    @Synchronized
    fun start(ctx: Context): Boolean {
        if (alive() && ping()) return true
        stop()
        val prefs = Prefs(ctx)
        // A fixed port keeps the page origin (and so its saved theme etc.) the same between launches.
        val p = freePort(prefs.port)
        if (p == 0) { lastError = "no free port on this phone"; return false }
        prefs.port = p
        port = p
        token = randomToken()
        val log = logFile(ctx)
        if (log.length() > 256 * 1024) log.renameTo(File(ctx.filesDir, "engine.log.1"))
        return try {
            val pb = ProcessBuilder(binary(ctx).path).redirectErrorStream(true)
                .redirectOutput(ProcessBuilder.Redirect.appendTo(log))
            pb.environment().putAll(env(ctx, prefs, token))
            pb.environment()["NATLAS_ADDR"] = "127.0.0.1:$p"
            proc = pb.start()
            val until = System.currentTimeMillis() + 10_000
            while (System.currentTimeMillis() < until) {
                if (!alive()) break
                if (ping()) { lastError = ""; return true }
                Thread.sleep(80)
            }
            lastError = tail(ctx, 6).ifBlank { "the engine did not start" }
            stop()
            false
        } catch (e: Exception) {
            lastError = e.message ?: e.toString()
            false
        }
    }

    @Synchronized
    fun stop() {
        proc?.let { it.destroy(); if (!it.waitFor(2, TimeUnit.SECONDS)) it.destroyForcibly() }
        proc = null
    }

    fun restart(ctx: Context): Boolean { stop(); return start(ctx) }

    private fun ping(): Boolean = try {
        val c = URL(url("/healthz")).openConnection() as HttpURLConnection
        c.connectTimeout = 300; c.readTimeout = 500
        val ok = c.responseCode == 200
        c.disconnect()
        ok
    } catch (_: Exception) { false }

    fun tail(ctx: Context, lines: Int): String = try {
        logFile(ctx).readLines().takeLast(lines).joinToString("\n")
    } catch (_: Exception) { "" }

    // One request to `libnatlas.so cli` (works with or without the app open).
    fun cli(ctx: Context, req: JSONObject, timeoutSec: Long = 15): JSONObject {
        val prefs = Prefs(ctx)
        return try {
            val pb = ProcessBuilder(binary(ctx).path, "cli").redirectErrorStream(true) // one stream: no pipe can fill up
            pb.environment().putAll(env(ctx, prefs, randomToken()))
            val pr = pb.start()
            pr.outputStream.use { it.write(req.toString().toByteArray()) }
            val out = pr.inputStream.bufferedReader().readText()
            if (!pr.waitFor(timeoutSec, TimeUnit.SECONDS)) { pr.destroyForcibly(); return fail("timed out") }
            val line = out.lineSequence().lastOrNull { it.trim().startsWith("{") }
            if (line == null) fail(out.ifBlank { "no answer from the engine" }.take(300)) else JSONObject(line)
        } catch (e: Exception) {
            fail(e.message ?: e.toString())
        }
    }

    private fun fail(msg: String) = JSONObject().put("status", 500).put("error", msg)
}
