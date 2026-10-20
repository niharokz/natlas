package app.natlas

// Folder picker → real path. The picker returns a tree id like
// "primary:Sync/notes/data"; the engine needs "/storage/emulated/0/Sync/notes/data".
object Paths {
    fun fromTreeId(docId: String): String? {
        val (volume, rel) = docId.split(":", limit = 2).let { if (it.size == 2) it[0] to it[1] else return null }
        val root = if (volume.equals("primary", ignoreCase = true)) "/storage/emulated/0" else "/storage/$volume"
        return if (rel.isBlank()) root else "$root/${rel.trim('/')}"
    }

    // What the folder looks like: a short line for Settings.
    fun describe(names: List<String>): Pair<Boolean, String> {
        val known = listOf("event.md", "subscription.md", "calendar.md", "health.md", "inventory.csv", "homelab.md")
        val found = known.filter { it in names }
        return when {
            names.isEmpty() -> false to "The folder is empty or not readable."
            found.isEmpty() -> false to "No Natlas files here (event.md, calendar.md, …). Pick the folder that holds them, e.g. …/notes/data."
            else -> true to "Found ${found.joinToString(", ")}"
        }
    }
}
