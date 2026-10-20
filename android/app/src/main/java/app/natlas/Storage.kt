package app.natlas

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Environment
import android.provider.DocumentsContract
import android.provider.Settings
import java.io.File

// File access ("All files access") and the folder checks.
object Storage {
    fun hasAccess(): Boolean = Environment.isExternalStorageManager()

    fun accessIntent(ctx: Context) = Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION, Uri.parse("package:${ctx.packageName}"))

    fun pickIntent() = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE).putExtra("android.provider.extra.SHOW_ADVANCED", true)

    // Tree URI from the picker → a path the engine can open.
    fun pathOf(uri: Uri): String? = try { Paths.fromTreeId(DocumentsContract.getTreeDocumentId(uri)) } catch (_: Exception) { null }

    // The sample data lives in the app's own folder: no permission needed.
    fun demoDir(ctx: Context): File = File(ctx.getExternalFilesDir(null) ?: ctx.filesDir, "demo")

    fun isDemo(ctx: Context, folder: String) = folder.isNotEmpty() && folder == demoDir(ctx).path

    // (looks right, one-line note)
    fun check(ctx: Context, folder: String): Pair<Boolean, String> {
        if (folder.isEmpty()) return false to "No folder chosen"
        val dir = File(folder)
        if (!isDemo(ctx, folder) && !hasAccess()) return false to "Allow file access first"
        if (!dir.isDirectory) return false to "The folder does not exist (moved or renamed?)"
        val names = dir.list()?.toList() ?: emptyList()
        return Paths.describe(names)
    }

    fun canStart(ctx: Context, folder: String) = check(ctx, folder).first && (hasAccess() || isDemo(ctx, folder))
}
