# Natlas for Android

Your Natlas lists on the phone, offline. The app reads and writes the same files your
server uses (the folder Syncthing keeps in sync), so a change on either side shows up on
the other after the next sync. Nothing leaves the phone: the engine listens only on
`127.0.0.1`, behind a per-launch token.

- **The same app as the web UI**: Today, every list, Calendar, Architecture, quick add, undo.
- **9 home-screen widgets**, resizable (see below). Tick a task straight from the widget;
  an **Undo** bar stays for 6 seconds.
- **Fingerprint / device lock or a PIN**, lock-again delay, hidden in recent apps.
- **Health from Samsung Health** (via Health Connect): every morning your steps, sleep, heart, weight and more go into `health.md`, with charts in Health → Daily and four health widgets.
- **Settings** for the folder, which lists show and in what order, start screen,
  theme, animations, widget style, and sync conflicts.

Android 12 or newer (API 31+).

## How it works

```
 Natlas app (Kotlin, no AndroidX)
 ├─ lock / setup / splash ── native views
 ├─ WebView ───────────────► 127.0.0.1:<port>  libnatlas.so  (the Go engine, phone mode)
 │   window.NatlasNative ◄── Settings page (#/phone) talks to the app
 └─ widgets ───── JSON ◄──── libnatlas.so cli  (snapshot · action · patch · quick · merge)
                                    │
                                    ▼
                  /storage/emulated/0/…/notes/data   ◄── Syncthing ──►  server
```

- The engine is the normal `natlas` binary, built for the phone and shipped as
  `jniLibs/<cpu>/libnatlas.so`. The app runs it as a program with `NATLAS_MODE=local`:
  no password (the app's lock protects it), plugins and sample data built in, no
  `natlas.yml` needed.
- Widgets never start the web server: they call `libnatlas.so cli` with one JSON
  request and draw from the answer ("snapshot"). A tick is the plugin's primary action
  (Done, Taken…), checked against the record's revision, so a stale widget cannot
  overwrite a newer change.
- Writes are atomic (temp file + rename). The app and its widgets share a lock folder,
  so two writes on the phone never interleave. Shared data files are never locked.

## Sync conflicts

When the phone and the server change the same file before Syncthing passes the change
on, Syncthing keeps both: `event.md` and `event.sync-conflict-20261019-101010-ABCDEFG.md`.
Natlas merges these **record by record**:

- records only in the copy are added back;
- a record changed on both sides keeps the file's version, and the other version waits
  in `.natlas/conflicts.yml`. The Today screen shows "1 record was changed on two
  devices"; tap it to compare field by field and choose **Keep this** or **Use other**;
- the copy is then deleted.

Only one device should merge: **the server** (`NATLAS_MAIN`, on by default there).
Settings → Sync → "This phone merges sync conflicts" stays off on the phone unless you
use Natlas without the server.

## Widgets

Long-press the home screen → Widgets → Natlas. All can be resized.

| Widget | Default | Smallest | Shows |
|---|---|---|---|
| Today | 4×2 | 2×2 | Overdue, today and next items with tick boxes |
| List | 4×2 | 2×2 | One list (you pick), open groups, tick boxes |
| Agenda | 4×2 | 3×1 | Calendar: today and coming days; one line at 4×1 |
| Month | 4×4 | 3×3 | Month grid with dots; ‹ › to change month, tap a day |
| Up next | 2×2 | 2×1 | The next date: "3 days · Alex's birthday" |
| Quick add | 4×1 | 1×1 | Add to a list (you pick) without opening the app |
| Number | 2×1 | 1×1 | One number (you pick): "₹555 per month" |
| Glance | 4×1 | 3×1 | All numbers of one list side by side |
| Clock | 4×2 | 3×2 | Time, date and the next two items |
| Steps | 2×2 | 1×1 | Steps ring against your goal; km and kcal beside it when wide |
| Sleep | 4×2 | 2×2 | Last night: hours, bedtime – wake, stages; the week's nights when wide |
| Steps week | 4×2 | 3×2 | The last 7 days as bars with the goal line and the daily average |
| Weight | 2×2 | 2×1 | Latest weight, change over 30 days, and the trend line |

Widget style: follow the phone, light, dark, AMOLED black, glass or clear; background
opacity; Natlas teal or the wallpaper colour (Material You); hide titles. Widgets refresh
on every change in the app, every 30 minutes, and just after midnight.

## Health (Samsung Health → Health Connect → health.md)

Samsung Health has no API of its own; it shares into **Health Connect**, which is built into Android 14+. Natlas reads Health Connect, read only.

1. Samsung Health → Settings → Health Connect: allow it to share steps, exercise, sleep, heart rate, weight, body composition (and anything else you track).
2. Natlas → Settings → Health → **Allow access**. Allow everything, plus **Access past data** (history older than 30 days) and **Access data in the background** (the morning sync with the app closed).
3. **Import all history** once (it walks back 90 days at a time; a few years take a minute or two).
4. **Sync every morning** at the time you choose (07:00 by default).

What a sync writes, in `daily:` of `health.md` (one record per day):

- the last 3 full days (late band syncs are picked up), and today with **last night's sleep only** (sleep belongs to the day you woke up; steps for today come with tomorrow's sync);
- every metric Health Connect has: `steps, km, floors, elev_m, kcal_active, kcal_total, active_min, workouts, exercise, sleep_h, sleep_start, sleep_end, deep_h, rem_h, light_h, awake_h, naps_h, hr_rest, hr_min, hr_avg, hr_max, hrv, spo2, resp_rate, weight, body_fat, height, bp_sys, bp_dia, glucose, water_l, kcal_eaten, protein_g, carbs_g, fat_g, sources`. A missing value is left out, never written as 0;
- the newest weight, body fat and height into the profile (`meta:`), with `last_updated`.

Steps, distance, floors and calories are counted per minute: when the phone, the band and Google Fit all saw the same walk, only the highest source counts for that minute (Health Connect's own daily total adds them up). Settings → Health shows each source's count for the last synced day.

A day that did not change is not rewritten, so Syncthing only moves real changes. The same run from your band and Strava is counted once. Goals: add `goal_steps`, `goal_sleep_h`, `goal_active_min` to the profile (Health → Profile).

If the phone was off at sync time, or background access is not allowed, the sync runs the next time you open Natlas.

## Build (on your server, with Docker)

Nothing to install but Docker. Run from the repo root:

```sh
cp android/.env.example android/.env     # set NATLAS_KEYSTORE_PASSWORD
./android/build.sh keystore              # once: creates $KEYS_HOME/.natlas-keys/natlas.jks - back it up
./android/build.sh                       # → android/dist/natlas-<version>.apk
```

- `./android/build.sh quick` skips the tests and builds for 64-bit phones only (fastest).
- `./android/build.sh debug` needs no key; it installs next to the release app.
- The first build downloads the Android SDK and Gradle into the builder image and
  `android/.cache` (a few GB); later builds take a minute or two.
- Updates must be signed with the same key. Lose the key and you must uninstall
  (losing nothing: your data lives in the notes folder, not in the app).

Version: the root `VERSION` file.

Signing keys live in `$KEYS_HOME/.natlas-keys/` (`KEYS_HOME` from your shell profile, default `~/.local/share/keys`), next to the other apps' keys. `NATLAS_KEYSTORE_DIR` in `android/.env` overrides it.

## Config folder

Settings → **Config folder** chooses where the app keeps its configuration:

- `settings.json` — these settings (folder, lists, lock mode, theme, widget style, health sync), written on every change. Choose the same folder after a reinstall or on a new phone and they come back. The PIN is never written there (a PIN lock comes back as device lock until you set the PIN again).
- `natlas.yml` and `plugins/` — optional, for the engine: your own plugin changes, read instead of the built-in ones.

Without a config folder everything stays inside the app and is lost when it is uninstalled. A folder inside your Syncthing notes (for example `notes/config/natlas/`) is also backed up to the server.

## Install and first start

1. Copy the APK to the phone and open it (allow "install unknown apps" for your file
   manager once).
2. Natlas asks for **All files access** - needed to read and write the Syncthing folder
   in place. Grant it.
3. **Choose folder**: pick the folder holding `event.md`, `calendar.md`… (for example
   `Sync/notes/data`). Or **Use sample data** to try it first.
4. Settings → Security: device lock (fingerprint, face, pattern) or a Natlas PIN.

## Troubleshooting

| Problem | Fix |
|---|---|
| "No Natlas files here" | Pick the `data` folder itself, not its parent. |
| A list is missing | Settings → Lists: switch it on. Missing file? It is created on the first save (calendar) or the list stays empty. |
| Widget shows "Open Natlas once" | Open the app once after install or update; widgets need one snapshot. |
| Widget is stale | Settings → Widgets → Refresh widgets now. Some phones stop background work for apps: set Natlas to "Unrestricted" battery use. |
| "Changed elsewhere, reload" | The file changed under you (sync). Reload and redo the edit. |
| Health: "No Health Connect permission yet" | Settings → Health → Allow access. To change it later: Health Connect settings. |
| Health: only 30 days imported | Allow "Access past data" in Health Connect, then Import history again. |
| Engine stopped | Settings → About → View log / Restart engine. |
