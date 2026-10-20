# Changelog

Version numbers restarted at **1.0.0** on 2026-10-20. Earlier releases are listed as 0.x: the Python version is 0.1, and the Go releases first published as 2.0.0 – 3.1.1 are 0.2.0 – 0.6.1.

## 1.0.0 - 2026-10-20

- **Version 1.0.0.** Numbers restart here; earlier releases are now 0.2.0 – 0.6.1 (see the top of this file). The Android app installs over 3.1.1 as an update.
- **Banned for days, not minutes.** Five wrong logins (or wrong device keys) in a day ban a public IP for 7 days (`NATLAS_MAX_FAILURES`, `NATLAS_BAN_DAYS`). Every request from it gets 403, and the ban survives restarts (`bans.yml`). Lift one with `natlas unban <ip>`; `natlas bans` lists them. Home/LAN addresses only get the 30-minute lockout.
- **Device key.** A login now lasts 12 hours (`NATLAS_SESSION_HOURS`). Put `NATLAS_DEVICE_KEY` in `.env` and enter it once under **This device** in a browser or the installed app: that browser then stays signed in until you log out, also after the next login. Changing the key signs every remembered browser out. Everyone is signed out once by this update.
- **Config folder.** The server keeps `natlas.yml` and `bans.yml` in `NATLAS_CONFIG_DIR` (default `./config`). The Android app has Settings → **Config folder** for `settings.json` (its settings, kept across reinstalls), `natlas.yml` and `plugins/`.
- **Android signing key** moved to `$KEYS_HOME/.natlas-keys` (default `~/.local/share/keys/.natlas-keys`).

## 0.6.1 - 2026-10-20

- **Steps no longer double-counted.** Health Connect keeps steps per app and per device, and its own daily total adds the phone, the band and Google Fit together (a 19k day showed as 34k). The phone app now counts each minute once, from the source with the most steps that minute, the way Samsung Health merges them. The same applies to distance, floors, elevation and calories; exercise minutes come from the de-duplicated workouts.
- **Settings → Health** shows what each source counted on the last synced day and the merged total, so you can compare with Samsung Health.
- After updating, tap **Import history again** once to correct past days.

## 0.6.0 - 2026-10-20

- **Health → Daily.** Your Samsung Health data, every day, in `health.md` (`daily:`): steps, distance, floors, calories, exercise and workouts, sleep with stages, heart rate (resting, min, avg, max), HRV, SpO2, breathing, weight, body fat, height, blood pressure, glucose, water and food.
  - **Today:** rings for steps, sleep and exercise against your goals; tiles with the change against your 30-day average and a 2-week sparkline; this week against last week.
  - **Trends:** any metric over 7 days, 30 days, 90 days, a year or everything; daily, weekly or monthly bars with the goal line; average, total, best and lowest day, goal-met days, change against the previous period.
  - **Sleep:** each night from bedtime to wake-up, split into deep / REM / light / awake; averages, bedtime consistency, longest and shortest night.
  - **Body:** weight and body fat (and blood pressure, glucose) lines with change, 7-day average, lowest and highest.
  - **Days:** the plain list, newest first.
- **Android app:** reads Health Connect (Samsung Health shares into it) every morning at the time you set, plus **Import all history** and **Sync now** (Settings → Health). The newest weight and body fat also update the profile.
- **Four health widgets:** Steps ring, Sleep, Steps week, Weight.
- **Plugins:** `charts:` (a dashboard over any dated list) and `sync:` (a list the phone fills, with `latest:` copying values into a record).
- **Faster:** an unchanged data file is parsed once and shared between the lists, the Today screen and the widget snapshot.

## 0.5.0 - 2026-10-19

- **Android app.** Natlas on the phone, offline, over the folder Syncthing keeps in sync. Build it with Docker: `./android/build.sh keystore` once, then `./android/build.sh`. See `android/README.md`.
  - The full app (Today, every list, Calendar, Architecture), with fingerprint / device lock or a PIN.
  - **9 resizable widgets:** Today, List, Agenda, Month, Up next, Quick add, Number, Glance, Clock. Tick tasks right on the widget, with Undo.
  - **Settings:** notes folder, which lists show and their order, start screen, lock, theme, animations, widget style (light, dark, AMOLED, glass, clear, opacity, wallpaper colour), sync.
- **Sync conflicts merged record by record.** When Syncthing leaves `event.sync-conflict-…md`, the server adds back records that exist only in the copy and deletes it. A record changed on both devices shows on the Today screen: compare and pick one. On by default on the server (`NATLAS_MAIN`).
- **Engine:** phone mode (`NATLAS_MODE=local`), `natlas cli` (JSON in, JSON out), plugins and sample data built into the binary (`natlas.yml` optional in phone mode).
- **Web app:** links to a record (`#/p/<plugin>/<collection>/r/<id>`), a new entry (`…/new`) or a calendar day (`#/p/calendar/day/<date>`).
- **Docker:** the image build now copies `plugins/` and `examples/` into the build stage (they are embedded). Run `docker compose build` before `check`.

## 0.4.0 - 2026-10-19

- **Architecture page.** A new `architecture` plugin shows one Markdown note (`homelab.md`) as a handbook: contents sidebar (a drawer on phones) that follows your scroll, numbered sections, search, tables, Obsidian-style callouts and Mermaid diagrams that follow the light/dark theme.
  - Edit the title and intro, or any section, with a Write / Preview switch; add a section below any other, move or delete it; undo as everywhere.
  - Saving a section rewrites only that section, so edits made in Obsidian to other sections at the same time are kept; the same section changed elsewhere gives "changed elsewhere, reload".
- **Plugins:** `format: markdown` (a note split into `front` + `sections`), `view: document`, and a `markdown` field type.
- **Vendored:** marked 15.0.12 (Markdown, ~70 KB, part of the app shell) and mermaid 11.16.1 (~3.5 MB, loaded only when a page has a diagram). The binary grows by about 3.6 MB.

## 0.3.1 - 2026-10-18

- **Calendar replaces Birthdays.** A new `calendar` plugin with month, week and schedule views, like Google Calendar:
  - tap a day to see it, tap again (or +) to add; tap an entry to edit, move or delete it; undo as everywhere;
  - quick add understands times: `Dinner with Riya fri 8pm`;
  - show or hide each calendar; colours by category (birthday, holiday, festival, event, reservation…);
  - swipe left / right on the month or week to change it.
  Your own entries live in `calendar-local.md` (created on the first save). Entries synced from Google Calendar into `calendar.md` show alongside, read-only, with an "Open in Google Calendar" button - Natlas never writes that file, so a nightly sync cannot undo your edits. Today's entries and the next 7 days' birthdays and holidays appear on the Today screen.
- **Removed:** the `birthdays` plugin and `examples/data/birthdays.md`. Replace `birthdays` with `calendar` in your `natlas.yml`.
- **Plugins:** `view: calendar` and `week_start`; a read-only collection may read its own `file:`; a `time` field type; `quick_add.time`; `link: { label, url }` buttons on records.
- **Today screen:** the same synced entry from two calendars (for example two holiday calendars) is listed once.

## 0.3.0 - 2026-10-07

- **About page.** `/` is now a public page describing Natlas, with install steps and a Sign in button. The app moved to **`/app`**. Reinstall the phone app once so its start page is `/app`.
- **Password hash.** `natlas hash-password` prints a PBKDF2-SHA256 `NATLAS_PASSWORD_HASH` for `.env`; a plain `NATLAS_PASSWORD` still works.
- **Sessions.** The cookie is now `__Host-natlas` over HTTPS and is bound to the password: changing the password (or the secret key) signs every device out. Everyone is signed out once by this update.
- **Login lockout fixed behind proxies.** The visitor IP now comes from `CF-Connecting-IP`, `X-Real-IP` or the last `X-Forwarded-For` entry. Before, a client could dodge the lockout by forging the first `X-Forwarded-For` entry. Failed logins are also slowed down, and the lockout table can no longer grow without limit.
- **Hardening.** HSTS, Permissions-Policy, COOP/CORP and a tighter CSP; same-origin check on login and logout; panic recovery; access log of every change (IP, method, path, status); idle and header limits; no directory listings under `/assets/`; file paths trimmed from error messages.
- **Ops.** `GET /healthz` for health checks, `natlas version`, `VERSION` build arg, `cap_drop: ALL` in compose.
- **Docs.** `SECURITY.md`; README and ARCHITECTURE updated.


## 0.2.0 - 2026-10-05

A rewrite. Data files keep their format; config and plugins are new.

- **Go instead of Python.** One static binary. The web UI and timezone data are built in. The Docker image is about 10 MB.
- **Plugins are a single `plugin.yml`.** Fields, dropdowns, list layout, groups, actions, quick add, Today-screen rules and numbers are all config. No per-plugin Python, HTML or JS.
- **New editing model.**
  - A compact list replaces the spreadsheet-style inline editing. Tapping a row opens a form (full-screen on phones, side panel on desktop).
  - Tap a badge to change it in place.
  - One-tap actions (Done, Tomorrow, Completed…), swipe gestures, long-press bulk edit.
  - Undo on every change.
  - Smart quick add (`Call bank tomorrow #todo !high`).
- **Today screen.** Overdue, today and next 7 days across all plugins, plus key numbers.
- **nss theme.** Light and dark follow the system; the six v1 themes are gone.
- **PWA.** Installable, plugin shortcuts, update prompt, offline read-only mode, `?nosw` escape hatch.
- **Safer file handling.**
  - Only the edited record is written.
  - Comments and unknown fields are kept; files are never re-sorted; reading never writes.
  - Stale edits are refused per record. Writes are atomic.
  - Values that PyYAML would misread are quoted.
- **Config validation.** `natlas check` reports every mistake at once. Unknown keys are errors.
- **Security.** Signed cookie, constant-time login check, IP lockout, same-origin JSON writes, CSP, non-root read-only container.

### Upgrading from the Python version (0.1)

- Secrets stay in `.env`. Create `natlas.yml` from `natlas.example.yml`; the v1 `tabs:` format no longer applies.
- `subscription.md` is now read and written as one `subscriptions:` list, which is the layout Taskmaster uses.
- Event priority is now `high` / `medium` / `low`. v1 wrote `normal`; existing values still display, and can be fixed once with
  `sed -i 's/^\(\s*\)priority: normal$/\1priority: medium/' event.md`.
