# Natlas architecture

> How Natlas works inside. It is meant to be shared in a design or debugging conversation instead of the whole source: share this file, plus `HOMELAB.md` for deployment details (that file is gitignored), plus only the files being changed.
> It contains no secrets and no personal paths, so it is safe to commit.
> Keep the **Changelog** at the bottom current.

## 1. Design rules

1. **Flat files, no database.** YAML for structured data, CSV for tables. One file per plugin; a read-only collection may read a second file (Calendar reads `calendar.md`, which a nightly sync rewrites in full). Natlas only ever writes the plugin's own file.
2. **Plugins are YAML, not code.** All behaviour comes from `plugin.yml`, and the core never mentions a specific plugin. A new feature should become a new field type, condition operator or action kind in the core, so every plugin can use it.
3. **Be a good neighbour to other writers.** Other programs (Taskmaster, the Telegram bot) edit the same files, so Natlas:
   - edits only the record that changed;
   - never sorts the file and never writes when it is only reading;
   - refuses stale edits;
   - writes atomically.
4. **YAML 1.1 compatibility.** Strings that PyYAML would misread are written single-quoted, the same way `yaml.safe_dump` does.
5. **Vendored dependencies.** Go modules are in `vendor/`; Alpine and nss are in `web/vendor/`. Neither the build nor the browser fetches anything from the internet.
6. **Fail at startup with clear messages.** Every config problem is reported at once, naming the file and key. At runtime, a broken data file shows as an error on that plugin only.

## 2. Request flow

```
browser ──► recoverer ─► accessLog ─► securityHeaders ─► net/http ServeMux
(Alpine, app.js)                   ├── /healthz ....................... plain "ok", no auth (Docker health check)
                                   ├── /api/login /logout /me ......... internal/auth
                                   ├── /api/app /dashboard /p/... ..... internal/api ──► internal/store ──► data file
                                   │                                        │               (yaml.Node / csv rows)
                                   │                                        └── internal/query (where, sort)
                                   └── / (about page) /app (the app)
                                       /assets/ /sw.js /manifest ...... web (go:embed)
```

The main pieces:

- **Process.** `cmd/natlas/main.go` loads the config, wires the routes and middleware, and shuts down gracefully. Subcommands: `natlas check` (validate the config), `natlas hash-password` (stdin → `NATLAS_PASSWORD_HASH=…`), `natlas health` (GET `/healthz`, for the Docker HEALTHCHECK) and `natlas version` (set with `-ldflags "-X main.version=…"`, Docker build arg `VERSION`).
- **Middleware** (outermost first):
  - `recoverer` turns a panic into a logged 500.
  - `accessLog` logs every non-GET request and every failed request (except 401/404) as `ip method path status duration`. It never logs bodies, cookies or query strings.
  - `securityHeaders` sets CSP (`'unsafe-eval'` only because Alpine compiles expressions; `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`), nosniff, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`, Permissions-Policy, COOP/CORP, and HSTS when cookies are Secure.
- **Server limits:** header 10 s, read and write 30 s, idle 120 s, headers capped at 64 KB, JSON bodies at 1 MB (login at 4 KB).
- **Error messages:** a 500 reaches the browser with absolute paths trimmed to file names (`publicMessage`); full detail goes to the log.
- **Startup config.** `internal/config` reads:
  - `Settings` from the environment;
  - `App` from `natlas.yml`;
  - each `Plugin` from `plugins/<id>/plugin.yml`.

  YAML is decoded strictly (`KnownFields`), so typos in keys are errors. `prepare()` fills defaults, resolves option lists, and compiles every `where` / `when` / `count` into a `query.Cond`.
- **No caching.** Every request reads the file fresh. Files are small, and this means a change made by another program shows up immediately.

## 3. Packages

| Package | Responsibility |
|---|---|
| `internal/config` | Types for `natlas.yml` and `plugin.yml`, defaults, validation. `Collection.Lookup()` describes fields, including the virtual anniversary keys, to the query engine. |
| `internal/store` | The `Doc` interface: `Items`, `Create`, `Update`, `Delete`, `Move`, `Top`, `Save`. Implemented by `yamlDoc`, `csvDoc` and `markdownDoc`. A `File` is one path plus its lock: `For(plugin)` is the plugin's own file, `ForCollection(plugin, coll)` the file a collection reads (`Plugin.FileOf`). `File.Change(fn)` handles lock → read → fn → save. |
| `internal/query` | The where language (`eq ne in nin lt lte gt gte empty contains`, plus `any` / `all` / `not`). Values are compared by field type. `Sort` is stable and puts empty values last. |
| `internal/api` | Generic handlers, input cleaning and validation (`values.go`), Today screen (`dashboard.go`: reads each file once; a read-only entry with the same title and date as one already listed is skipped), quick-add parser (`quick.go`: date words, `#tag`, `!priority`, and `8pm` / `20:30` for a `time` role). |
| `internal/auth` | Signed session cookie (`user|expiry|kind`, HMAC-SHA256), named `__Host-natlas` when Secure (`natlas_session` for plain-http testing). Kind `s`: `NATLAS_SESSION_HOURS` (12 h). Kind `d`: 400 days, renewed daily while used, signed with a key that also includes `NATLAS_DEVICE_KEY`; issued when the browser holds the device cookie (`__Host-natlas-device`, an HMAC mark set by `POST /api/device {key}`; `DELETE` forgets it). The base signing key is HMAC(secret key, username + credential), so a new password or secret invalidates every session; a new device key only the `d` ones. Bans (`bans.go`): 5 failed logins or device keys per public IP within 24 h → banned `NATLAS_BAN_DAYS` (7), stored in `<NATLAS_STATE_DIR>/bans.yml` (default: the config folder), re-read when it changes so `natlas unban` takes effect at once; `Guard` answers 403 to every request from a banned IP except `/healthz`. Private, loopback and link-local IPs are never banned (30 min in-memory lockout instead). Password is plain (`NATLAS_PASSWORD`) or a PBKDF2-SHA256 hash (`NATLAS_PASSWORD_HASH`, format `pbkdf2-sha256:iter:salt:key`, no `$` so compose needs no quoting), compared in constant time. Lockout: 5 failures per IP in 30 minutes, plus a 400 ms delay on every failure; the table is held in memory and pruned when it reaches 10 000 IPs. Client IP (`ClientIP`, when `NATLAS_TRUST_PROXY`): `CF-Connecting-IP`, then `X-Real-IP`, then the *last* `X-Forwarded-For` entry, otherwise the TCP peer. Login and logout also go through the same-origin check. |
| `internal/merge` | Syncthing conflict copies (`<stem>.sync-conflict-YYYYMMDD-HHMMSS-XXXXXXX<ext>`). `Scan` merges each copy into its file record by record through `store.File.Change`: records only in the copy are appended, records that differ keep the file's version and the copy's version is queued in `<data>/.natlas/conflicts.yml` (stable `conflictID`), then the copy is deleted. Files only read by read-only collections are skipped (their writer owns them). `List` / `Resolve(id, kept|other)` back the Today card and its dialog. |
| `internal/store/upsert.go` | `Upserter` (YAML docs): set fields on the record matching a key (the date) or insert a new one in key order; one parse, one write for a whole batch. Unchanged input means no write. |
| `internal/dates` | `Today()` in the container's TZ; parsing with a Go layout (ISO is always accepted too); tokens such as `today+7`; `CycleDays`, using the same rules as Taskmaster's `advance()`; `NextAnniversary`, where 29 Feb falls back to 28 Feb as in Taskmaster's `daily_update.py`; `Clock` normalises `6:30pm` / `18:30` to `HH:MM` (a bare `8` or a dotted `12.10` without am/pm is not a time). |
| `web` | Embeds the UI. `GET /` is the public about page (`landing.html` + `landing.css`, no JavaScript); `GET /app` is the app (`index.html`); `/app/` redirects. `{{VERSION}}` (a hash of all web files) busts caches; `{{RELEASE}}` is the binary version; `{{TITLE}}` is HTML-escaped. `/assets/` refuses directory listings and `.go` files. Generates the PWA manifest (`start_url` and `id` `/app`), with one shortcut per plugin. `NATLAS_WEB_DIR` serves the files from disk during development. |

## 4. Storage details

### YAML (`store/yaml.go`)

- **Parsing.** The file is parsed into a `yaml.Node` tree. A top-level `updated:` key is removed while editing and written back as a fresh `updated: <now>\n\n` header on save, but only if the file had one.
- **Collections.** A collection is found by dotted `path` (for example `runs.planned`). `kind: list` expects a sequence and `kind: record` a mapping. Missing parts of the path are created on the first write.
- **Node to value.** `!!int` and `!!float` become `float64`, `!!bool` becomes `bool`, `!!null` becomes `nil`, and everything else, including timestamps and `65:30`, becomes a **string**. Sequences become `[]any`.
- **Update.** Only the given keys have their value nodes replaced, and line and head comments are carried over. New keys are appended. The record's position never changes.
- **Create.** The new mapping gets `id` first, then the fields in schema order. It goes at `new_at` (end or start), or at an explicit index (used by undo-delete).
- **Writing values.** `valueNode()` single-quotes any string that matches PyYAML's implicit resolvers (bool, int including sexagesimal, float, null, timestamp, merge). Whole floats are written as ints (`1200`, not `1200.0`).
- **Output.** The file is encoded with a 2-space indent. Lists come out as `  - item` (go-yaml style) rather than PyYAML's `- item`. Both are valid, and Taskmaster rewrites the file in its own style overnight anyway.

### Markdown (`store/markdown.go`, `format: markdown`)

- **Parsing.** Optional front matter (`---` … `---`/`...`, YAML mapping), then the intro (text before the first section), then sections: a line starting with `## ` outside a fenced code block (```` ``` ```` or `~~~`) starts one. CRLF files are read as LF and written back as CRLF.
- **Collections.** `path: sections` (list) → records `{title, body}`; ids are slugs of the titles (unique in order, `-2` for repeats) and are never stored. `path: front` (record) → front-matter keys + `intro`. Any other path is an error.
- **Writing.** Every section keeps its raw text; only edited ones (and the front block, if edited) are rebuilt as `## Title` + blank line + body. Exactly one blank line is ensured before each heading and one newline at the end of the file. Clearing a front-matter value removes the key. Create/undo-delete insert at an index; move swaps neighbours.
- **Conflicts.** The revision is the hash of `{title, body}`, so an Obsidian edit to one section never blocks saving another; a renamed or deleted section gives 404 "reload".

### CSV (`store/csv.go`)

- **Raw cells.** Cells are kept as raw text, and only the edited row is re-encoded, so untouched rows stay byte-identical.
- **Line endings.** CRLF is preserved if the file used it.
- **Columns.** Unknown columns are kept. Columns for new fields are appended. An `id` column is added at the front the first time a record without an id is edited.
- **Types.** Number and money cells become `float64` (empty becomes `nil`); `list` cells are split on `|`.

### Identity and revisions

- **Ids.** A record's id is its `id` field. If it has none, the id is `slug(id_from)`, made unique in file order, and computed in memory on every read. The id is persisted on the record's first edit.
- **Revisions.** `Rev` is the first 12 hex characters of the SHA-1 of the record's JSON (sorted keys). Every change except create and move sends the revision it saw. A mismatch returns **409**, and the UI says "changed elsewhere, reload". Move only swaps positions, so it does not check the revision.
- **Locking.** There is one mutex per file, which serialises Natlas's own writes. Other programs are not locked out; the revision check is what protects their changes.

## 5. API

| Method | Path | Body → result |
|---|---|---|
| GET | `/api/app` | Title, currency, locale, username, and the plugin definitions (as JSON from the config types). |
| GET | `/api/dashboard` | `{agenda:{overdue,today,upcoming}, stats:[{plugin,widgets,error?}], today}` |
| GET | `/api/p/{p}/{c}` | `{records, groups:[{label,collapsed,ids}], options:{field:[values]}, note}`. Each record has `_id`, `_rev`, `_actions` (the action ids that apply) and anniversary virtual fields. Records come sorted by `list.sort` (unless `order: manual`); grouping happens on the server. |
| POST | `/api/p/{p}/{c}` | `{values, id?, at?}` → 201 `{record}`. Every field is written; missing ones get their default or null. |
| POST | `/api/p/{p}/{c}/quick` | `{text}` → 201 `{record}` |
| POST | `/api/p/{p}/{c}/bulk` | `{items:[{id,rev,values?}], action? \| values?}` → `{results:[{id,error?,record?,before?}]}`. The file is saved once, and only if something changed. |
| PATCH | `/api/p/{p}/{c}/{id}` | `{rev, values}` (only the changed fields) → `{record, before}` |
| DELETE | `/api/p/{p}/{c}/{id}?rev=` | → `{deleted, id, at}` (enough to undo) |
| POST | `/api/p/{p}/{c}/{id}/do/{action}` | `{rev, values}` (the answers for `ask:` fields) → `{record, collection, before}` |
| POST | `/api/p/{p}/{c}/{id}/move` | `{dir:-1\|1}` (only when `order: manual`) |

Status codes:
- **401** not logged in.
- **403** read-only collection, or a cross-origin write.
- **404** unknown plugin or record.
- **409** stale revision, or a duplicate id.
- **415** a write that is not JSON.
- **422** `{error, fields:{key:msg}}`.

Input cleaning (`clean` / `convert`):
- Empty input becomes `nil`.
- Dates are accepted as ISO, as a token, or in the field's format, and stored in the field's format.
- A select value outside the list is refused, unless the field has `allow_new`, takes options from `data`, or the record already held that value.

Undo works entirely on the client, using what the server returns:
- **Set or patch:** a PATCH with the `before` values of the changed fields.
- **Delete:** a create with the same `id` and `at`.
- **Create:** a delete.
- **Bulk:** a bulk call with per-item values.
- **Move actions** (`move_to`) cannot be undone.

## 6. Frontend (`web/`)

- **Pages.** `/` is a static about page (features, install steps, configuration, plugin example, security) with an Install and a Sign in button, styled with the nss tokens in `landing.css`. The app lives at `/app`; its login screen links back to `/`.
- **Structure.** One Alpine component, `natlas`, in `app.js`, and one `index.html`. Load order: `app.js` (defer), then `alpine.min.js` (defer). The component registers on `alpine:init`.
- **Routing.**
  - Screens use the hash: `#/today`, `#/p/{plugin}/{collection}`.
  - The edit sheet is a `<dialog>`. Opening it pushes a history entry, so the phone's Back button closes it.
- **Rendering.** Everything comes from the plugin definition: `list.title`, `subtitle`, `badges`, `search`, `filters`; `actions` with `primary`; `quick_add`; `summary`; `link` (a button in the sheet).
- **Calendar view** (`view: calendar`, the `calendar` section of `app.js`):
  - All collections of the plugin are loaded and flattened (`calAll`) into entries with `date`, `end` (inclusive, ignored if before `date`), `time`, `cat` (category → CSS class `cat-*`, Google-like colours in `natlas.css`) and `cal` (the `calendar` field, else the collection label). `calByDay` buckets them per ISO day, all-day first.
  - Month (6×7 grid; on phones events shrink to colour bars and the selected day is listed below), Week (7 columns, stacked on phones), Schedule (days with entries, 90 days at a time).
  - Tapping a day selects it; tapping the selected day, `+`, or the FAB opens the normal sheet for the first writable collection with that date. Entries open the normal sheet (read-only for synced ones, empty fields hidden). Swipe on the grid changes the period.
  - The chosen view and hidden calendars are kept in localStorage (`natlas_cal_view`, `natlas_cal_hidden`).
- **Document view** (`view: document`, the `document` section of `app.js`):
  - Loads the `front` record and the `sections` list. Layout: a sticky contents column from 1100 px (a drawer opened from a sticky bar below that), a hero card (title, subtitle, intro, "N sections · reviewed …"), then numbered sections. `docSpy` (rAF-throttled scroll handler) highlights the section at the top.
  - `md()` renders with marked (GFM; raw HTML allowed - single user, and the CSP blocks inline scripts) and caches by text. `mdEnhance()` runs after `x-html`: `> [!kind] Title` blockquotes become `.callout.<tone>`, tables get a scroll wrapper (a header row of empty cells is dropped), outside links open in a new tab, and ```` ```mermaid ```` blocks become `.mermaid-box` placeholders.
  - `drawDiagrams()` loads `vendor/mermaid.min.js` once (script tag, versioned URL), initialises it with `securityLevel: 'strict'` and theme variables read from the `--nss-*` tokens, and renders each box; errors show the message and the source. A theme switch re-renders every diagram.
  - The sheet gets a `markdown` field editor (Write / Preview) and is wider (`dialog.sheet.wide`); "add below" sends `at` with the create request.
- **Gestures** (`pressStart` / `pressMove` / `pressEnd`):
  - Rows use `touch-action: pan-y`.
  - A horizontal drag over 80 px runs the primary action (right) or opens the form (left).
  - Holding for 480 ms selects the row.
- **Alpine gotcha.** Alpine *calls* any function an expression evaluates to. Never write `x-show="obj.fn"`; store a boolean instead (see `toast.hasUndo`).
- **Teardown.** Templates use optional chaining and keep `sheet.coll` set after the sheet closes, so Alpine never re-evaluates bindings against null while tearing a template down.
- **Styling.**
  - `natlas.css` uses only `--nss-*` tokens.
  - `vendor/nss.min.css` is a subset build of nss 3.0: tokens, base, typography, code, tables, forms, interactive, a11y. Layout and patterns are left out because their structural selectors would fire on app markup.
  - nss 3.0 has a bug: its shared field rule outranks its `select` rule and removes the dropdown arrow. `natlas.css` puts the arrow back.
- **Theme.** `auto` follows the system; `dark` or `light` sets `data-theme` on `<html>`. localStorage holds only UI choices: the theme and the calendar view and hidden calendars.
- **Service worker** (`sw.js`):
  - The app shell is cached per `{{VERSION}}`.
  - `GET /api/*` is network-first. When the network fails, the last good copy is served with an `X-Natlas-Offline` header, and the UI then shows "offline" and turns editing off.
  - Writes never go through the service worker, and `respondWith` always resolves to a real Response.
  - When a new version is waiting, an "Update" toast appears.
  - `?nosw` unregisters the worker and clears its caches.

### Charts and sync (0.6.0)

- `config/charts.go`: `Collection.Charts` (`date`, `goals`, `rings`, `tiles`, `trends`, `sleep`, `body`; a `Metric` may be a bare field name) and `Collection.Sync` (`source: health_connect`, `key`, `latest: {collection, set}` with `{field}` / `{today|layout}` templates). Both validated in `prepareCharts`.
- `api/sync.go`: the `upsert` cli op → `store.Upserter`, then `applyLatest` (newest non-empty value per template; `today`-only fields change only together with a real value).
- `api/snapshot.go` `chartEntry`: per charts collection, the last 35 days, goals (profile `goal_<field>` over plugin.yml), newest values; such collections are left out of `lists`.
- `store.File.View()`: a read-only parse shared while the file's SHA-1 is unchanged (lists, dashboard, snapshot). Writes still go through `Change`, which always re-reads. Cuts a snapshot over a 10-year `health.md` from ~0.7 s to ~0.15 s.
- Web: the `charts` section of `app.js` draws everything from the list's records (HTML bars, SVG lines, rings, sparklines); points are kept outside Alpine (`CH`) and a tap reads the nearest one. Sleep-stage colours are the validated categorical slots 1-4, per theme.
- Android widgets (`HealthWidgets.kt`): charts are ALPHA_8 masks sized to the widget and tinted with the day/night colour pairs (small bitmaps, theme switches without redraw).

## 7. Adding features

| Need | Where |
|---|---|
| A new kind of data | A new `plugins/<id>/plugin.yml`. No code. |
| A new field type | `config.fieldTypes`, `api.convert`, `store` (if it needs special storage), `app.js` `display()` / `inputType()` and the sheet template. |
| A new data format (like `markdown`) | A `Doc` implementation in `store`, `File.Format` / `Plugin.Format` (`format:` key), and its collection rules in `config.prepare`. |
| A new view (like `calendar`) | `config.Plugin.View` (plus validation in `prepare`), a section in `app.js`, a `<template x-if>` in the plugin view of `index.html`, styles in `natlas.css`. |
| A new condition operator | `query.validOps` and `fieldCond.Match`. |
| A new action kind | `config.Action` (plus validation in `prepare`) and `api.action` / `runSet` / `runMove`. |
| A new data format (for example JSON) | Implement `store.Doc`, then pick it by file extension in `File.Read` and `Plugin.Format`. |

## 8. Testing

Run `go test ./...` (Go uses `vendor/` automatically). The tests cover:

- **store:** file untouched on read; comments and unknown fields kept; YAML 1.1 quoting; create, delete, undo and move; nested paths and record collections; CSV byte-identical rows and CRLF.
- **query:** operators, date tokens, select order, sorting.
- **dates:** anniversaries (including the leap day), cycles, formats, clock times.
- **config:** shipped plugins are valid (calendar files resolved); mistakes are collected and reported together; unknown keys are rejected; `file:` only on read-only collections; a calendar view needs `title` and `date`.
- **api:** an end-to-end flow (login, Done, stale-revision 409, 422, quick add, synced calendar read-only 403 and untouched, own calendar entries add / bad time 422 / quick add with time / delete, cross-origin 403, health move, dashboard without errors), `calendar-local.md` created on first save, the quick-add parser (dates and times), and path trimming in error messages.
- **api (health sync):** upsert inserts in date order (older than all, newer than all), updates only changed fields, copies weight/body fat into the profile with `last_updated`, writes nothing on a repeat, rejects bad input (no date 400, non-number 422, no `sync:` 400); snapshot `charts` (goals from the profile, latest values, daily left out of lists). `NATLAS_BENCH=1` times a 10-year backfill.
- **api (phone):** `Snapshot` in local mode with embedded plugins (lists without closed groups or document/calendar plugins, Done action present, calendar window); `Exec` tick → `undo` values → patch restores; stale tick 409; quick add (events, calendar with a time); merge op; formatting helpers (Indian grouping, 12-hour clock).
- **merge:** copy detection, added records, queued differences, resolve both ways, read-only files skipped.
- **auth:** cookie attributes, tampered cookies, sessions ending on a password change, PBKDF2 hashes, per-IP lockout, client-IP extraction (forged X-Forwarded-For ignored).
- **web:** `/` and `/app` routes, title escaping, `/app/` redirect, no directory listings or source files, manifest and service-worker version.

UI changes are checked by hand with `scripts/dev.sh`, on a phone-sized viewport and on desktop.

Android: `./android/build.sh` runs `go vet`, `go test`, then the Kotlin unit tests (`PinTest`, `PathsTest`) before building the APK. Kotlin is compiled with warnings as errors.

## 9. Phone mode and the Android app

The Android app (`android/`) is a thin Kotlin shell around this same engine; see `android/README.md` for the user side.

```
MainActivity ── native lock / setup / splash
   └─ WebView ──► 127.0.0.1:<port>/local?t=<token>&next=/app#/… ──► cookie natlas_local ──► the normal /app
         window.NatlasNative (Bridge.kt) ◄──► #/phone settings page, setBars, toasts (natlasNativeEvent)
Widgets (9 AppWidgetProviders) ──► libnatlas.so cli  {"op":"snapshot"|"action"|"patch"|"quick"|"merge"}
WidgetRouter (invisible activity) ── tick → action → Undo bar 6 s → patch
```

- **Phone mode** (`NATLAS_MODE=local`, `config.Settings.Local`): needs `NATLAS_TOKEN` (32+ characters, new per launch) instead of a user and password; `GET /local?t=…&next=…` sets `natlas_local` and redirects; `auth.User` accepts only that cookie. `natlas.yml` is optional: `NATLAS_DATA_DIR` and `NATLAS_PLUGINS` (comma list) replace it, and plugins come from `plugins.FS` (`go:embed`) when `plugins/` is not on disk. `examples.Data` (embedded sample data) backs "Use sample data".
- **`natlas cli`** reads one JSON request on stdin and prints one JSON answer. `api.Server.Exec` runs the same HTTP handlers in-process (`Server.call`, `httptest`), so the widgets get exactly the web app's rules (revisions, validation, actions). `snapshot` (`snapshot.go`) is ready-to-show text: `format.go` ports `display()` from `app.js` (relative dates, money, 12-hour times, Indian grouping for `-IN` locales). `action` returns `undo`: the old values of the fields the action changed.
- **Locks between natlas processes.** The app's server and each widget `cli` are separate processes: `store.LockDir` (`NATLAS_LOCK_DIR`) adds an `flock` on `<dir>/<sha1(path)>.lock` around `File.Change`. These lock files live in the app's private folder; shared data files are never locked, so Syncthing and other writers are unaffected.
- **Health sync** (0.6.0): `android/…/health/HealthReader.kt` reads Health Connect (framework API, Android 14+): steps, distance, floors, elevation and calories from raw records merged per minute by `MinuteMerge` (best source per minute; Health Connect's aggregate adds phone + band + Google Fit, 0.6.1), daily aggregates for heart rate, water and nutrition, records for sleep (wake-date rule, longest session = the night, the rest naps), measurements (latest or mean per day) and workouts (band + Strava duplicates overlapping ≥ 50 % counted once). `HealthSync` sends them as `natlas cli {"op":"upsert"}`; `HealthJob` (JobScheduler) runs at the set time, `catchUp` on app open if it was missed. Rate-limit errors back off and retry.
- **Who merges conflicts.** `NATLAS_MAIN` (default `true` on the server, `false` in phone mode) starts `mergeLoop` (every 15 s). Exactly one device should merge, otherwise both would rewrite the same file.
- **Android shell** (no AndroidX, minSdk 31): `Engine` starts `libnatlas.so` from `nativeLibraryDir` (`useLegacyPackaging`) with a fixed port saved in prefs and `TZ` from the phone; `Lock` uses `BiometricPrompt` (`BIOMETRIC_WEAK | DEVICE_CREDENTIAL`) or a PBKDF2 PIN (`Pin`); widgets use `RemoteViews` size maps (2×1 … 5×5), `RemoteCollectionItems` for lists, `setOnCheckedChangeResponse` for ticks and day/night colour pairs.

## Changelog

- **1.0.0 (2026-10-20).** Version reset (old 2.0.0–3.1.1 → 0.2.0–0.6.1; Android versionCode epoch 100000). `auth/bans.go` multi-day IP bans with `Guard`, `natlas bans` / `unban`; session kinds `s` (12 h) / `d` (until logout, device key, `POST/DELETE /api/device`); `NATLAS_CONFIG_DIR` mounted at `/config` (natlas.yml + bans.yml); Android `AppConfig` (config folder: settings.json, natlas.yml, plugins/); keys under `$KEYS_HOME`.

- **0.6.0 (2026-10-20).** Health from the phone: `charts:` and `sync:` collection options, `store` upsert, cli `upsert`, snapshot `charts`, parse cache for reads, Health → Daily (Today / Trends / Sleep / Body / Days), Android Health Connect reader + morning job + history import, four health widgets.

- **0.5.0 (2026-10-19).** Android app and sync-conflict merge: phone mode (`NATLAS_MODE=local`, token cookie, embedded plugins and sample data), `natlas cli` + `api.Exec` / `Snapshot` / `format.go`, `internal/merge` + `/api/conflicts` + Today card and dialog + `NATLAS_MAIN`, `store.LockDir` flock between natlas processes, `#/phone` settings page and deep links in the web app, `android/` (Kotlin shell, 9 widgets, Docker build).

- **0.4.0 (2026-10-19).** Architecture page: `store/markdown.go` (`format: markdown`), `view: document`, `markdown` field type, vendored marked + mermaid (lazy), `plugins/architecture`, `examples/data/homelab.md`, store + API + config tests.

- **0.3.1 (2026-10-18).** Calendar replaces Birthdays:
  - `plugins/calendar`: `mine` (writable, `calendar-local.md`) + `google` (read-only, own `file: calendar.md`, written nightly by the sync job), merged in a Google-Calendar-like month / week / schedule view.
  - Core additions, usable by any plugin: `view: calendar`, `week_start`, collection `file:` (read-only only; `move_to` across files refused), field type `time`, `quick_add.time`, collection `link`.
  - Startup log: a missing file read by a read-only collection is a WARNING; a missing writable file is a note (created on the first save).
  - Today screen: duplicate synced entries (same title and date) listed once; widget errors have paths trimmed.
  - Removed `plugins/birthdays` and `examples/data/birthdays.md`.

- **0.3.0 (2026-10-07).** Production hardening and an about page:
  - `/` is now a public about page; the app moved to `/app` (manifest `start_url` too, so reinstall the phone app once).
  - Password can be a PBKDF2 hash (`natlas hash-password`); sessions are bound to the password; `__Host-` cookie.
  - Client IP read safely behind Cloudflare and Caddy (the old code trusted the first, forgeable `X-Forwarded-For` entry).
  - Failure delay and a memory cap on the lockout table; login and logout are same-origin only.
  - More security headers (HSTS, Permissions-Policy, COOP/CORP, tighter CSP), panic recovery, access log, `/healthz`, server idle and header limits, no directory listings, paths trimmed from error messages.
  - `natlas version` and a `VERSION` Docker build arg; container drops all capabilities.
  - `SECURITY.md`; auth and web tests.

- **0.2.0 (2026-10-05).** Rewrite in Go:
  - Plugins are now only YAML: no per-plugin Python, HTML or JS.
  - Editing moved to a list plus an edit sheet, with badge menus, one-tap actions, swipe, bulk edit, undo and quick add.
  - New Today screen.
  - Theme is now nss (light and dark).
  - PWA with offline read-only mode.
  - Record-level conflict detection.
  - Subscriptions use Taskmaster's flat `subscriptions:` list. v1 split it into `active:` / `cancelled:`, so the layout flipped back and forth every night.
  - Event priority values now match Taskmaster (`high` / `medium` / `low`).
