# Natlas architecture

> The complete technical reference: every file, every function, every setting. Read this first
> instead of the whole source; then open only the files you change. It holds no secrets and no
> host paths (those live in `.env` and in the private, gitignored `HOMELAB.md`). User-facing
> steps are in `README.md` and `android/README.md`. Keep §13 and the Changelog current.

## 1. What Natlas is

A self-hosted app for personal lists kept as plain files (YAML, CSV, Markdown) in a notes folder.
One Go binary serves the web app (`/app`) and a public about page (`/`); the same binary runs
inside the Android app in **phone mode**. Every tab is a **plugin**, and a plugin is only a
`plugin.yml`: the core never mentions a specific plugin.

## 2. Design rules

1. **Flat files, no database.** YAML for structured data, CSV for tables, Markdown for notes. One file per plugin; a read-only collection may read a second file (Calendar reads `calendar.md`, rewritten nightly by another job). Natlas writes only the plugin's own file.
2. **Plugins are YAML, not code.** A new feature becomes a field type, condition operator, action kind, view or collection option in the core, usable by every plugin.
3. **Be a good neighbour to other writers** (Kronos jobs, a launcher, Obsidian, Syncthing): edit only the changed record, never sort, never write on read, refuse stale edits (revisions), write atomically, keep comments, unknown fields and the file's own layout. No locks on shared data files.
4. **YAML 1.1 compatible.** Strings PyYAML would misread are single-quoted, as `yaml.safe_dump` does.
5. **Vendored.** Go modules in `vendor/`; Alpine, marked, mermaid and nss in `web/vendor/`. Neither the build nor the browser fetches anything at run time.
6. **Fail at startup, clearly.** Every config problem is reported at once with file and key (`natlas check`). At run time a broken data file only shows an error on that plugin.
7. **Modular.** One concern per file: Go packages by layer, the web app as one file per screen (`views/`), per feature (`js/`, `css/`); the APK build behind one script and one image.
8. **No paths or secrets in the repo.** Everything personal comes from `.env` (see §4); `scripts/check-public.sh` blocks a push that would publish one.

## 3. Repository map

```
VERSION                 X.Y.Z — the one version: server binary, Docker image, APK (versionName/Code)
CHANGELOG.md            user-facing changes per version ("## X.Y.Z - date"; release notes come from it)
README.md               user guide: install, configuration, plugins, releases, security
ARCHITECTURE.md         this file
SECURITY.md             how to report a problem; supported versions
LICENSE                 MIT
.env.example            every setting Natlas reads, commented (copy to .env or into a shared .env)
natlas.example.yml      app settings template (copy to <config>/natlas.yml)
docker-compose.yml      the server container: only NATLAS_* and TZ reach it; read-only, no capabilities
Dockerfile              server image: build the binary (vendored, no network), run it FROM scratch
.dockerignore           keeps the server image context to Go sources, web, plugins, examples
.gitignore              secrets, runtime state (config/), build output (dist/, jniLibs), patch/
.gitattributes          line endings
.gitlab-ci.yml          CI: check on every push; on a vX.Y.Z tag build the APK (gtlbrunner) and release (§10)
.gitlab/                issue templates (Bug, Feature) and the merge request template
go.mod, vendor/         module "natlas"; gopkg.in/yaml.v3 vendored
cmd/natlas/             the binary: main.go (commands), serve.go, commands.go, middleware.go (§5)
internal/config/        Settings (.env), App (natlas.yml), Plugin (plugin.yml) + validation, charts options
internal/store/         Doc interface and the YAML, CSV and Markdown implementations; File (lock, cache)
internal/query/         the where / when / count language and sorting
internal/api/           HTTP API, Today screen, layout, quick add, phone snapshot + cli, health upsert
internal/auth/          login, signed sessions, device key, demo login, lockout and IP bans, passwords
internal/dates/         today, date tokens and formats, cycles, anniversaries, clock times
internal/merge/         record-by-record merge of Syncthing conflict copies
web/                    browser app, embedded in the binary (§7)
plugins/<id>/plugin.yml the plugins: events, subscriptions, calendar, health, inventory, bookmarks,
                        locations, architecture; plugins.go embeds them for phone mode
examples/data/          sample data for every plugin; examples.go embeds it (demo login, phone app)
android/                the Android app: Kotlin shell, widgets, Health Connect (§9)
docker/builder/         Dockerfile of natlas-builder: JDK 17, Android SDK 34, Go (§10)
scripts/                build-in-container.sh, upload-release.sh, check-version.sh, release.sh,
                        check-public.sh, dev.sh (§10, §11)
── private / generated, never committed ──
.env                    settings and secrets (usually a symlink to the shared homelab .env)
HOMELAB.md              how this deployment runs (paths, neighbours); gitignored
config/                 runtime state: natlas.yml, layout.yml, bans.yml, demo/
dist/                   build output (dist/release/natlas-X.Y.Z.apk, SHA256SUMS)
patch/                  patch zips applied on the server
```

## 4. Configuration

### 4.1 Environment (`.env`)

`internal/config.LoadSettings` reads only these. Compose passes only `NATLAS_*` and `TZ`.

| Variable | Default | Meaning |
|---|---|---|
| `NATLAS_USERNAME` | — (required) | the single user |
| `NATLAS_PASSWORD_HASH` | — | `pbkdf2-sha256:iter:salt:key` from `natlas hash-password` (wins over the plain password) |
| `NATLAS_PASSWORD` | — | plain password, used only when no hash is set |
| `NATLAS_SECRET_KEY` | — (required, 32+) | HMAC key of the session cookie |
| `NATLAS_DEVICE_KEY` | empty | 32+; a browser given it stays signed in until logout |
| `NATLAS_SESSION_HOURS` | `12` | login life without the device key |
| `NATLAS_MAX_FAILURES`, `NATLAS_BAN_DAYS` | `5`, `7` | failures per day before a public IP is banned, and for how long |
| `NATLAS_DEMO_USERNAME`, `NATLAS_DEMO_PASSWORD` | empty | read-only demo login over the sample data (name ≠ `NATLAS_USERNAME`) |
| `NATLAS_TRUST_PROXY` | `true` | client IP from `CF-Connecting-IP` / `X-Real-IP` / last `X-Forwarded-For` |
| `NATLAS_COOKIE_SECURE` | `true` | `false` only for plain-http tests (cookie loses `__Host-`) |
| `NATLAS_MAIN` | `true` (phone `false`) | this copy merges Syncthing conflict copies |
| `NATLAS_DATA_DIR` | `data_dir` of natlas.yml | data folder (compose mounts it at `/data`) |
| `NATLAS_CONFIG_DIR` | `./config` | compose only: host folder mounted at `/config` |
| `NATLAS_CONFIG` | `/config/natlas.yml` in the image | path of natlas.yml (optional in phone mode) |
| `NATLAS_STATE_DIR` | folder of `NATLAS_CONFIG` | where `layout.yml`, `bans.yml`, `demo/` live |
| `NATLAS_PLUGINS_DIR` | `/app/plugins` in the image | plugin folders; embedded plugins when missing |
| `NATLAS_PLUGINS` | `plugins:` of natlas.yml | comma list of plugins (phone mode) |
| `NATLAS_WEB_DIR` | embedded | serve `web/` from disk (UI development) |
| `NATLAS_ADDR` | `:8080` | listen address |
| `NATLAS_MODE`, `NATLAS_TOKEN` | — | `local` = phone mode, unlocked by the per-launch token (32+) |
| `NATLAS_LOCK_DIR` | — | folder for flock files between natlas processes (phone) |
| `NATLAS_NETWORK` | `homelab` | compose only: the reverse proxy's Docker network |
| `PUID`, `PGID`, `TZ` | `1000`, `1000`, `UTC` | container user; timezone for "today" |

### 4.2 Files

| File | Written by | Holds |
|---|---|---|
| `<config>/natlas.yml` | you (`natlas.example.yml`) | `title`, `currency`, `locale`, `data_dir`, `plugins` (order, per-plugin `file`/`title` overrides), shared `lists` |
| `<state>/layout.yml` | Customize (`PUT /api/layout`) | menu order, hidden lists, Today blocks |
| `<state>/bans.yml` | `auth.Bans` | banned IPs and recent failures (`natlas bans` / `unban`) |
| `<state>/demo/` | `serve.demoMux` at start | a fresh copy of `examples/data` for the demo login |
| `<data>/.natlas/conflicts.yml` | `merge` | queued sync conflicts |
| `plugins/<id>/plugin.yml` | you | a plugin (README → Writing a plugin) |

## 5. Server

### 5.1 Process (`cmd/natlas`)

| File | What |
|---|---|
| `main.go` | `main` dispatches `natlas [check \| cli \| hash-password \| health \| version \| bans \| unban <ip\|all>]`, else `serve`; `load` reads Settings + App. `version` is set by `-ldflags "-X main.version=…"` from `VERSION`. |
| `serve.go` | `serve`: routes (`/healthz`, `api.Server.Register`, `web.Handler`), demo switch (`demoMux`: a second read-only `api.Server` over `<state>/demo` for demo sessions), `mergeLoop` (every 15 s when `NATLAS_MAIN`), startup log lines, graceful shutdown. `warnMissingFiles`, `writeDemo`. |
| `commands.go` | `cli` (one JSON request from the phone widgets → `api.Server.Exec`), `hashPassword`, `health` (Docker HEALTHCHECK), `bans`. |
| `middleware.go` | `recoverer` → `accessLog` → `auth.Guard` → `securityHeaders` around every request. |

### 5.2 Request flow

```
browser ──► recoverer ─► accessLog ─► auth.Guard (banned IP → 403) ─► securityHeaders ─► ServeMux
(Alpine)                                  ├── /healthz ....................... "ok", no auth
                                          ├── /api/login /logout /me /device . internal/auth
                                          ├── /api/app /dashboard /layout /p/… internal/api ─► internal/store ─► data file
                                          │     (demo session: the demo mux, read-only)       └─ internal/query
                                          └── / (about) /app (the app) /assets/ /sw.js /manifest  web
```

- **Middleware.** `recoverer` turns a panic into a logged 500. `accessLog` logs every non-GET and every failed request (not 401/404) as `ip method path status duration`, never bodies, cookies or query strings. `securityHeaders`: CSP (`'unsafe-eval'` only for Alpine; `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`), nosniff, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`, Permissions-Policy, COOP/CORP, HSTS when cookies are Secure.
- **Limits.** Header 10 s, read and write 30 s, idle 120 s, headers 64 KB, JSON bodies 1 MB (login 4 KB).
- **Errors.** A 500 reaches the browser with absolute paths trimmed (`publicMessage`); details go to the log.
- **Config.** `internal/config` reads Settings (environment), App (`natlas.yml`) and each Plugin (`plugins/<id>/plugin.yml`), decoding strictly (`KnownFields`); `prepare()` fills defaults, resolves option lists and compiles every `where` / `when` / `count` into a `query.Cond`.
- **Reads.** Every request reads the file (small files; another program's change shows at once). `store.File.View()` shares one parse while the file's SHA-1 is unchanged; writes always re-read.

### 5.3 Packages

| Package | Responsibility |
|---|---|
| `internal/config` | Types for `natlas.yml` and `plugin.yml`, defaults, validation. `Collection.Lookup()` describes fields, including the virtual anniversary keys, to the query engine. |
| `internal/store` | The `Doc` interface: `Items`, `Create`, `Update`, `Delete`, `Move`, `Top`, `Save`. Implemented by `yamlDoc`, `csvDoc` and `markdownDoc`. A `File` is one path plus its lock: `For(plugin)` is the plugin's own file, `ForCollection(plugin, coll)` the file a collection reads (`Plugin.FileOf`). `File.Change(fn)` handles lock → read → fn → save. |
| `internal/query` | The where language (`eq ne in nin lt lte gt gte empty contains`, plus `any` / `all` / `not`). Values are compared by field type. `Sort` is stable and puts empty values last. |
| `internal/api` | Generic handlers, input cleaning and validation (`values.go`), Today screen (`dashboard.go`: reads each file once; a read-only entry with the same title and date as one already listed is skipped), quick-add parser (`quick.go`: date words, `#tag`, `!priority`, and `8pm` / `20:30` for a `time` role). |
| `internal/auth` | Signed session cookie (`user|expiry|kind`, HMAC-SHA256), named `__Host-natlas` when Secure (`natlas_session` for plain-http testing). Kind `s`: `NATLAS_SESSION_HOURS` (12 h). Kind `x`: the demo login (`NATLAS_DEMO_USERNAME`), its own HMAC key, `IsDemo(r)` routes its `/api/` requests to the demo server; never a device session. Kind `d`: 400 days, renewed daily while used, signed with a key that also includes `NATLAS_DEVICE_KEY`; issued when the browser holds the device cookie (`__Host-natlas-device`, an HMAC mark set by `POST /api/device {key}`; `DELETE` forgets it). The base signing key is HMAC(secret key, username + credential), so a new password or secret invalidates every session; a new device key only the `d` ones. Bans (`bans.go`): 5 failed logins or device keys per public IP within 24 h → banned `NATLAS_BAN_DAYS` (7), stored in `<NATLAS_STATE_DIR>/bans.yml` (default: the config folder), re-read when it changes so `natlas unban` takes effect at once; `Guard` answers 403 to every request from a banned IP except `/healthz`. Private, loopback and link-local IPs are never banned (30 min in-memory lockout instead). Password is plain (`NATLAS_PASSWORD`) or a PBKDF2-SHA256 hash (`NATLAS_PASSWORD_HASH`, format `pbkdf2-sha256:iter:salt:key`, no `$` so compose needs no quoting), compared in constant time. Lockout: 5 failures per IP in 30 minutes, plus a 400 ms delay on every failure; the table is held in memory and pruned when it reaches 10 000 IPs. Client IP (`ClientIP`, when `NATLAS_TRUST_PROXY`): `CF-Connecting-IP`, then `X-Real-IP`, then the *last* `X-Forwarded-For` entry, otherwise the TCP peer. Login and logout also go through the same-origin check. |
| `internal/merge` | Syncthing conflict copies (`<stem>.sync-conflict-YYYYMMDD-HHMMSS-XXXXXXX<ext>`). `Scan` merges each copy into its file record by record through `store.File.Change`: records only in the copy are appended, records that differ keep the file's version and the copy's version is queued in `<data>/.natlas/conflicts.yml` (stable `conflictID`), then the copy is deleted. Files only read by read-only collections are skipped (their writer owns them). `List` / `Resolve(id, kept|other)` back the Today card and its dialog. |
| `internal/store/upsert.go` | `Upserter` (YAML docs): set fields on the record matching a key (the date) or insert a new one in key order; one parse, one write for a whole batch. Unchanged input means no write. |
| `internal/api/layout.go` | `GET/PUT /api/layout`: `layout.yml` in the state dir (menu order, hidden, Today blocks `agenda` / `glance` / `list` / `note`). Read through `fit()`: unknown plugins dropped, new ones appended, broken blocks skipped. `{reset:true}` deletes the file. `Server.LayoutFile == ""` (demo) = defaults, not editable. `Server.ReadOnly` (demo) turns every non-GET route into 403. |
| `internal/dates` | `Today()` in the container's TZ; parsing with a Go layout (ISO is always accepted too); tokens such as `today+7`; `CycleDays`, using the same rules as Taskmaster's `advance()`; `NextAnniversary`, where 29 Feb falls back to 28 Feb as in Taskmaster's `daily_update.py`; `Clock` normalises `6:30pm` / `18:30` to `HH:MM` (a bare `8` or a dotted `12.10` without am/pm is not a time). |
| `web` | Embeds the UI. `GET /` is the public about page (`landing.html` + `landing.css`, no JavaScript); `GET /app` is the app (`index.html` with its `{{> views/…}}` parts expanded by `render`); `/app/` redirects. `{{VERSION}}` (a hash of all web files) busts caches; `{{RELEASE}}` is the binary version; `{{TITLE}}` is HTML-escaped; `{{DEMO}}` is the demo-login hint. `/assets/` refuses directory listings, `.go` files and `views/`. Generates the PWA manifest (`start_url` and `id` `/app`), with one shortcut per plugin. `NATLAS_WEB_DIR` serves the files from disk during development. |

## 6. Storage and API

### YAML (`store/yaml.go`)

- **Parsing.** The file is parsed into a `yaml.Node` tree. A top-level `updated:` key is removed while editing and written back as a fresh `updated: <now>\n\n` header on save, but only if the file had one.
- **Collections.** A collection is found by dotted `path` (for example `runs.planned`). `kind: list` expects a sequence and `kind: record` a mapping. Missing parts of the path are created on the first write.
- **Node to value.** `!!int` and `!!float` become `float64`, `!!bool` becomes `bool`, `!!null` becomes `nil`, and everything else, including timestamps and `65:30`, becomes a **string**. Sequences become `[]any`.
- **Update.** Only the given keys have their value nodes replaced, and line and head comments are carried over. New keys are appended. The record's position never changes.
- **Create.** The new mapping gets `id` first, then the fields in schema order. It goes at `new_at` (end or start), or at an explicit index (used by undo-delete).
- **Writing values.** `valueNode()` single-quotes any string that matches PyYAML's implicit resolvers (bool, int including sexagesimal, float, null, timestamp, merge). Whole floats are written as ints (`1200`, not `1200.0`).
- **Output.** The file is encoded with a 2-space indent, then `restyle()` puts back what yaml.v3 cannot keep: top-level lists that were written unindented (`key:\n- a`, per key, found at parse time) stay unindented, and one-line rows written `- { a: 1 }` keep the inner spaces. New rows copy flow style from their siblings (`matchStyle`: lists inline, strings double-quoted).
- **`id_field: none`.** For files another app owns (Rahi's bookmarks and locations): ids are only derived (`slug(id_from)`), never written on create or first edit, and empty fields are not written either. YAML lists only.

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

### API

| Method | Path | Body → result |
|---|---|---|
| GET | `/api/app` | Title, currency, locale, username, and the plugin definitions (as JSON from the config types). |
| GET / PUT | `/api/layout` | `{menu, hidden, home:[{type, title?, sections?, plugin?, coll?, group?, search?, limit?, text?}]}`; PUT `{reset:true}` restores the defaults. |
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

## 7. Web app (`web/`)

### 7.1 Files

```
web.go          Handler: pages (render: {{> …}} includes, placeholders), /assets/, manifest, version hash
index.html      the app skeleton: head (css/, js/ in order), login + app shell, rail, bottom bar, More sheet,
                selection bar, badge menu, toasts; one {{> views/…}} line per screen
views/          login.html · today.html · customize.html · phone.html (#/phone, Android settings) ·
                plugin.html (header, tabs, filters, quick add, record / summary / list) and
                plugin/document.html · plugin/calendar.html · plugin/charts.html · sheet.html · dialogs.html
js/             the Alpine component "natlas", one part per feature (table below); app.js merges them
css/            base · lists · calendar · document · phone · charts · customize (loaded in this order)
landing.html    the public about page (no JavaScript) + landing.css
sw.js           service worker
vendor/         alpine.min.js, marked.min.js, mermaid.min.js (lazy), nss.min.css (subset), LICENSES.txt
icons/          favicon, touch and PWA icons, icon.svg
```

**Parts.** `js/base.js` holds the constants (`ICONS`, badge tones, `CH` chart points, `ApiError`) and `part(name, make)`. Each other file calls `part()` with a factory returning its state and methods; `js/app.js` (last, before Alpine) merges the factories with property descriptors (getters survive) into `Alpine.data('natlas')` and warns if two parts define the same key. Every method is on the one component, so a part may call any other part's methods through `this`.

| File | Contents |
|---|---|
| `js/core.js` | state, the API wrapper (session expiry, offline, errors) and routing (#/today, #/p/<plugin>/<coll>, #/customize). State: `ready`, `authed`, `user`, `busy`, `loginForm`, `loginError`, `app`, `route`, `today`, `todayLoaded`, `layout`, `cz`, `demo`, `data`, `offline`, `search`, `filters`, `showFilters`, `summaryBy`, `collapsed`, `quick`, `selected`, `menu`, `sheet`, `ask`, `cal`, `ch`, `doc`, `mdPreview`, `_mdCache`, `_mermaid`, `phoneLog`, `toasts`, `moreOpen`, `theme`, `updateReady`, `native`, `phone`, `device`, `healthProgress`, `conflicts`, `_press`, `_suppressClick`, `_undone`, `_toastSeq`, `_sw`. Functions: `selecting`, `init`, `loadApp`, `api`, `login`, `setDevice`, `openDevice`, `saveDevice`, `forgetDevice`, `logout`, `applyRoute`, `reload`, `plugin`, `collOf`, `curPlugin`, `curColl`, `cur`, `pluginOfColl`, `load`, `_refresh`. |
| `js/today.js` | the Today screen (agenda, numbers), layout.yml (menu order, Today blocks) and the Customize page. Functions: `loadToday`, `todayTitle`, `agendaSections`, `menuPlugins`, `glanceStats`, `blockRows`, `blockTitle`, `listColls`, `czStart`, `czMove`, `czToggle`, `czAdd`, `czSetColl`, `czGroups`, `czSection`, `czSave`, `agendaSub`, `openFromAgenda`, `runAgendaAction`, `fmtWidget`. |
| `js/lists.js` | list screens: search, filters, groups, summary, display formatting, dates. Functions: `activeFilterCount`, `clearFilters`, `visibleRecords`, `groupedRecords`, `isCollapsed`, `toggleGroup`, `summaryRows`, `field`, `fieldLabel`, `visibleFields`, `subtitle`, `badges`, `isMuted`, `display`, `fmtField`, `text`, `money`, `parseDate`, `iso`, `daysFromToday`, `relDays`, `fmtDate`. |
| `js/interact.js` | gestures (swipe, long-press), selection and bulk edits, badge menus, one-tap actions, the ask dialog. Functions: `pressStart`, `pressMove`, `pressEnd`, `pressCancel`, `onContextMenu`, `rowClick`, `toggleSelect`, `clearSelection`, `selectedCount`, `bulkActions`, `bulkFields`, `bulkAction`, `bulkSet`, `_bulk`, `openMenu`, `pickMenu`, `optionsFor`, `primaryAction`, `runAction`, `_doAction`, `patch`, `_changedBack`, `afterChange`, `fail`, `quickAdd`, `_delete`, `openAsk`, `submitAsk`. |
| `js/calendar.js` | month / week / schedule view for plugins with `view: calendar`. Functions: `isCalendar`, `loadCalendar`, `calLoaded`, `calNotes`, `calAll`, `calEvents`, `calCalendars`, `toggleCal`, `calByDay`, `_d`, `_shift`, `_weekStart`, `calWeekdays`, `calDay`, `calMonth`, `calMonthDays`, `calSub`, `calWeek`, `calSchedule`, `calPicked`, `calTitle`, `calSetView`, `calMove`, `calToday`, `calPick`, `calDateLabel`, `calTime`, `calWhen`, `calCatClass`, `calWritable`, `calCanAdd`, `calNew`, `calOpen`, `calTouch`, `linkFor`. |
| `js/document.js` | a Markdown note as numbered sections (`view: document`): contents, edit, move, Mermaid. Functions: `isDocument`, `docColl`, `docLoaded`, `docAll`, `docFront`, `docSections`, `docGo`, `docSpy`, `docEdit`, `docEditFront`, `docAdd`, `docMeta`, `md`, `mdEnhance`, `loadMermaid`, `_dark`, `drawDiagrams`, `redrawDiagrams`. |
| `js/charts.js` | charts over a dated list (`charts:`, Health → Daily): Today, Trends, Sleep, Body tabs. Functions: `isCharts`, `chShowList`, `chTabs`, `chSetTab`, `chLoadGoals`, `chDays`, `chNum`, `chGoal`, `chLast`, `chLatest`, `chRange`, `chAvg`, `chShift`, `chEnd`, `chFmt`, `chTick`, `chDay`, `chWhen`, `chRings`, `chRingSVG`, `chTiles`, `chSpark`, `chWeek`, `chDelta`, `chTrendMetrics`, `chCurMetric`, `chSpan`, `chBuckets`, `chTrendHTML`, `chTrendStats`, `chScale`, `chFrame`, `chBarsHTML`, `chLineHTML`, `chPick`, `chLeave`, `esc`, `chSleep`, `chNights`, `chNightTip`, `chSleepAxis`, `chSleepStats`, `chBodyCards`, `chSigned`. |
| `js/sheet.js` | the edit form (new / edit / read-only), toasts and undo. Functions: `openRecord`, `openNew`, `_formValues`, `_showSheet`, `closeSheet`, `closeSheetQuiet`, `sheetClosed`, `sheetTitle`, `isLocked`, `inputType`, `otherFields`, `sheetWide`, `sheetActions`, `saveSheet`, `reloadSheet`, `deleteRecord`, `moveRecord`, `toast`, `dropToast`, `undo`. |
| `js/phone.js` | the Android app (window.NatlasNative): settings page (#/phone), sync conflicts. Functions: `nativeCall`, `loadPhone`, `setPhone`, `phoneToggle`, `setMotion`, `motionOn`, `showLog`, `onNative`, `loadConflicts`, `openConflicts`, `curConflict`, `resolveConflict`. |
| `js/shell.js` | service worker and update prompt, theme, preferences, icons. Functions: `setupServiceWorker`, `applyUpdate`, `cycleTheme`, `applyTheme`, `_getPref`, `_setPref`, `icon`. |

**Includes.** `web.render` replaces a whole line `{{> views/x.html}}` with that file (nested up to 3 deep). Parts keep their own indentation, so the expanded page is byte-identical to a single file. `/assets/views/` is not served.

### 7.2 Behaviour

- **Pages.** `/` is a short static about page (features, lists, install steps, the latest-APK link). The app lives at `/app`; its sign-in form shows the demo login (`{{DEMO}}`, a tap fills the form) and links back to `/`.
- **Routing** (hash): `#/today`, `#/p/{plugin}/{collection}`, `#/customize`, `#/phone` (Android only). The edit sheet is a `<dialog>`; opening it pushes a history entry, so Back closes it.
- **Layout.** `loadApp` fetches `/api/layout`; `menuPlugins()` orders the rail, bottom bar and More sheet. The Today screen loops over `layout.home`: `agendaSections(b)`, `glanceStats(b)`, `blockRows(b)` (list blocks load their collection in `loadToday`), `md(b.text)`. `#/customize` edits a copy (`cz`) and saves it with one PUT.
- **Demo.** `/api/me` and the login reply carry `demo`; the app marks every collection read-only (no FAB, quick add, Save, actions), hides agenda buttons and shows a banner. The server refuses writes anyway.
- **Rendering** comes from the plugin definition: `list.title`, `subtitle`, `badges`, `search`, `filters`; `actions` with `primary`; `quick_add`; `summary`; `link`.
- **Calendar** (`view: calendar`, `js/calendar.js`): all collections flattened (`calAll`) into entries with `date`, `end`, `time`, `cat` (CSS `cat-*`) and `cal`; `calByDay` buckets them. Month (6×7; colour bars on phones), Week, Schedule (90 days at a time). Tap a day to select, again (or `+`, FAB) to add. View and hidden calendars in localStorage (`natlas_cal_view`, `natlas_cal_hidden`).
- **Document** (`view: document`, `js/document.js`): `front` record + `sections` list; sticky contents from 1100 px (drawer below), hero card, numbered sections, `docSpy` scroll highlight. `md()` renders with marked (GFM) and caches; `mdEnhance()` adds callouts (`> [!kind]`), table wrappers, external-link targets and Mermaid boxes; `drawDiagrams()` loads mermaid once with `securityLevel: 'strict'` and nss colours, re-rendered on theme change.
- **Gestures** (`js/interact.js`): rows use `touch-action: pan-y`; a horizontal drag over 80 px runs the primary action (right) or opens the form (left); holding 480 ms selects.
- **Alpine gotchas.** Alpine *calls* a function an expression evaluates to: never `x-show="obj.fn"`, store a boolean (see `toast.hasUndo`). Templates use optional chaining and keep `sheet.coll` set after closing so teardown never evaluates against null.
- **Styling.** Only `--nss-*` tokens. `vendor/nss.min.css` is a subset of nss 3.0 (tokens, base, typography, code, tables, forms, interactive, a11y); its shared field rule hides the select arrow, so `css/lists.css` puts it back.
- **Theme.** `auto` follows the system; `dark` / `light` set `data-theme` on `<html>`. localStorage holds only UI choices.
- **Service worker** (`sw.js`): the shell (page, `css/`, `js/`, vendor, icons) is cached per `{{VERSION}}`; `GET /api/*` is network-first and the last good answer is served offline with `X-Natlas-Offline` (the UI turns editing off); writes never pass through it; a waiting version shows an Update toast; `?nosw` unregisters it.

## 8. Charts and health sync

- `config/charts.go`: `Collection.Charts` (`date`, `goals`, `rings`, `tiles`, `trends`, `sleep`, `body`; a `Metric` may be a bare field name) and `Collection.Sync` (`source: health_connect`, `key`, `latest: {collection, set}` with `{field}` / `{today|layout}` templates). Both validated in `prepareCharts`.
- `api/sync.go`: the `upsert` cli op → `store.Upserter`, then `applyLatest` (newest non-empty value per template; `today`-only fields change only together with a real value).
- `api/snapshot.go` `chartEntry`: per charts collection, the last 35 days, goals (profile `goal_<field>` over plugin.yml), newest values; such collections are left out of `lists`.
- `store.File.View()`: a read-only parse shared while the file's SHA-1 is unchanged (lists, dashboard, snapshot). Writes still go through `Change`, which always re-reads. Cuts a snapshot over a 10-year `health.md` from ~0.7 s to ~0.15 s.
- Web: `js/charts.js` draws everything from the list's records (HTML bars, SVG lines, rings, sparklines); points are kept outside Alpine (`CH`) and a tap reads the nearest one. Sleep-stage colours are the validated categorical slots 1-4, per theme.
- Android widgets (`HealthWidgets.kt`): charts are ALPHA_8 masks sized to the widget and tinted with the day/night colour pairs (small bitmaps, theme switches without redraw).

## 9. Phone mode and the Android app

The Android app (`android/`) is a thin Kotlin shell (no AndroidX, minSdk 31, package `app.natlas`) around this same engine; user steps in `android/README.md`.

```
MainActivity ── native lock / setup / splash
   └─ WebView ──► 127.0.0.1:<port>/local?t=<token>&next=/app#/… ──► cookie natlas_local ──► the normal /app
         window.NatlasNative (Bridge.kt) ◄──► #/phone settings page, setBars, toasts (natlasNativeEvent)
Widgets (9 AppWidgetProviders) ──► libnatlas.so cli  {"op":"snapshot"|"action"|"patch"|"quick"|"merge"}
WidgetRouter (invisible activity) ── tick → action → Undo bar 6 s → patch
```

- **Phone mode** (`NATLAS_MODE=local`, `config.Settings.Local`): needs `NATLAS_TOKEN` (32+ characters, new per launch) instead of a user and password; `GET /local?t=…&next=…` sets `natlas_local` and redirects; `auth.User` accepts only that cookie. `natlas.yml` is optional: `NATLAS_DATA_DIR` and `NATLAS_PLUGINS` (comma list) replace it, and plugins come from `plugins.FS` (`go:embed`) when `plugins/` is not on disk. `examples.Data` (embedded sample data) backs "Use sample data".
- **`natlas cli`** reads one JSON request on stdin and prints one JSON answer. `api.Server.Exec` runs the same HTTP handlers in-process (`Server.call`, `httptest`), so the widgets get exactly the web app's rules (revisions, validation, actions). `snapshot` (`snapshot.go`) is ready-to-show text: `format.go` ports `display()` from `js/lists.js` (relative dates, money, 12-hour times, Indian grouping for `-IN` locales). `action` returns `undo`: the old values of the fields the action changed.
- **Locks between natlas processes.** The app's server and each widget `cli` are separate processes: `store.LockDir` (`NATLAS_LOCK_DIR`) adds an `flock` on `<dir>/<sha1(path)>.lock` around `File.Change`. These lock files live in the app's private folder; shared data files are never locked, so Syncthing and other writers are unaffected.
- **Health sync** (0.6.0): `android/…/health/HealthReader.kt` reads Health Connect (framework API, Android 14+): steps, distance, floors, elevation and calories from raw records merged per minute by `MinuteMerge` (best source per minute; Health Connect's aggregate adds phone + band + Google Fit, 0.6.1), daily aggregates for heart rate, water and nutrition, records for sleep (wake-date rule, longest session = the night, the rest naps), measurements (latest or mean per day) and workouts (band + Strava duplicates overlapping ≥ 50 % counted once). `HealthSync` sends them as `natlas cli {"op":"upsert"}`; `HealthJob` (JobScheduler) runs at the set time, `catchUp` on app open if it was missed. Rate-limit errors back off and retry.
- **Who merges conflicts.** `NATLAS_MAIN` (default `true` on the server, `false` in phone mode) starts `mergeLoop` (every 15 s). Exactly one device should merge, otherwise both would rewrite the same file.
- **Android shell** (no AndroidX, minSdk 31): `Engine` starts `libnatlas.so` from `nativeLibraryDir` (`useLegacyPackaging`) with a fixed port saved in prefs and `TZ` from the phone; `Lock` uses `BiometricPrompt` (`BIOMETRIC_WEAK | DEVICE_CREDENTIAL`) or a PBKDF2 PIN (`Pin`); widgets use `RemoteViews` size maps (2×1 … 5×5), `RemoteCollectionItems` for lists, `setOnCheckedChangeResponse` for ticks and day/night colour pairs.

### 9.1 Android files

| File | What |
|---|---|
| `android/settings.gradle.kts`, `build.gradle.kts`, `gradle.properties`, `gradlew`, `gradle/wrapper/` | Gradle project (Android Gradle Plugin, Kotlin) |
| `android/app/build.gradle.kts` | app module: versionName = `VERSION` (`-PnatlasVersion`), versionCode = 100000 + major·10000 + minor·100 + patch; release signing only from the environment (`NATLAS_KEYSTORE`, `NATLAS_KEYSTORE_PASSWORD`, `NATLAS_KEY_ALIAS`, `NATLAS_KEY_PASSWORD`); Kotlin warnings are errors; `useLegacyPackaging` so `libnatlas.so` can be executed |
| `app/src/main/AndroidManifest.xml` | activities, widget providers, the health job, permissions (all files, Health Connect reads, background health) |
| `java/app/natlas/NatlasApp.kt` | the Application class (process-wide setup) |
| `MainActivity.kt` | entry: lock, first-run setup, splash, the WebView on the local engine, deep links, folder pickers |
| `Engine.kt` | starts `libnatlas.so` (server with a per-launch token, fixed port, phone TZ) and runs `natlas cli` requests |
| `Bridge.kt` | `window.NatlasNative` for the `#/phone` settings page (settings, folder, health, widgets, log) |
| `Prefs.kt` | the app's settings (folder, enabled lists, lock, theme, widget style, health sync) |
| `AppConfig.kt` | the optional config folder: `settings.json`, `natlas.yml`, `plugins/`, `layout.yml` |
| `Paths.kt` | data-folder paths and the list of known data files |
| `Storage.kt` | all-files access checks |
| `Lock.kt`, `LockView.kt`, `Pin.kt` | app lock: BiometricPrompt (biometric or screen lock) or a PBKDF2 PIN; the PIN pad |
| `SetupView.kt` | first start: choose the data folder or use the sample data |
| `QuickAddActivity.kt` | quick add from a widget or shortcut without opening the app |
| `Refresh.kt` | takes a new snapshot (`cli snapshot`) and redraws every widget |
| `Ui.kt` | small native view helpers |
| `health/HealthReader.kt` | reads Health Connect into one record per day (§8) |
| `health/MinuteMerge.kt` | per-minute merge of steps, distance, floors, calories: the best source per minute |
| `health/HealthSync.kt` | morning job (`HealthJob`, JobScheduler), "Sync now", history import → `cli upsert` |
| `widget/Widgets.kt` | the widget providers (Today, List, Agenda, Month, Up next, Quick add, Number, Glance, Clock) |
| `widget/HealthWidgets.kt` | Steps, Sleep, Steps week, Weight widgets (ALPHA_8 chart masks) |
| `widget/Render.kt` | builds the widgets' RemoteViews from the snapshot (size maps, lists, tick boxes) |
| `widget/Look.kt` | widget style: theme, opacity, colours |
| `widget/Snap.kt` | the stored snapshot JSON and helpers |
| `widget/MonthIds.kt` | view ids of the month grid cells |
| `widget/WidgetConfig.kt` | the configure screen (pick a list or a number) |
| `widget/WidgetReceiver.kt` | broadcasts: refresh every 30 min, after midnight, on changes |
| `widget/WidgetRouter.kt` | invisible activity: tick → action → 6 s Undo bar → patch |
| `app/src/test/…` | `PinTest`, `PathsTest`, `MinuteMergeTest` (JUnit, run by the build) |
| `res/layout/w_*.xml`, `p_*.xml` | widget layouts and their previews |
| `res/xml/w_*_info.xml`, `shortcuts.xml`, `network_security_config.xml` | widget provider info, launcher shortcuts, cleartext only to 127.0.0.1 |
| `res/drawable/`, `mipmap-anydpi-v26/`, `values*/` | icons, shapes, colours (day/night), themes, strings |
| `app/src/main/jniLibs/` | generated by the build (the engine per CPU); never committed |

## 10. Build and release

The APK is built only in Docker, by **gtlbrunner** (a dockerised GitLab runner on the server; its own repository documents it). Natlas provides the app side of its contract:

| Path | Role |
|---|---|
| `VERSION` | `X.Y.Z`; the tag must be `vX.Y.Z` |
| `docker/builder/Dockerfile` | image `natlas-builder:<tag>`: JDK 17, Android SDK 34 + build-tools 34.0.0, Go 1.24; no source, no keys |
| `scripts/build-in-container.sh [ci\|quick\|debug]` | runs from the source root inside the image: Go vet + tests, `libnatlas.so` for arm64-v8a / armeabi-v7a / x86_64, Gradle unit tests + `assembleRelease`, `apksigner verify`, then `dist/release/natlas-X.Y.Z.apk` + `SHA256SUMS`. Caches under `$BUILD_CACHE` (home, Gradle, Android, Go). Key from `$KEY_DIR`: `natlas.jks` + `keystore.env` (only `NATLAS_KEYSTORE_PASSWORD`, `NATLAS_KEY_ALIAS`, `NATLAS_KEY_PASSWORD` are read, without evaluating the file). Never uploads. |
| `scripts/upload-release.sh` | CI only: uploads `natlas-X.Y.Z.apk` as `natlas-vX.Y.Z.apk` and `SHA256SUMS` to the generic package `natlas/vX.Y.Z` (`CI_JOB_TOKEN`), writes `release-notes.md` from the CHANGELOG section |
| `scripts/check-version.sh [vX.Y.Z]` | VERSION is X.Y.Z, CHANGELOG has `## X.Y.Z - date`, the tag matches |
| `.gitlab-ci.yml` | `check` stage on shared runners: `test` (gofmt, vet, tests), `versions`, `public`. On a tag: `release-files` (tags `[natlas-app]`, `image: natlas-builder:1`, `BUILD_CACHE=/build-cache`, `KEY_DIR=/keys`), then `release` (release-cli: the GitLab release with permanent links `…/-/releases/permalink/latest/downloads/natlas.apk` and `SHA256SUMS`) |
| `scripts/release.sh "vX.Y.Z text"` | the maintainer's one command: VERSION, CHANGELOG section if missing, `check-version`, local tests when Go is new enough, `check-public`, commit everything, tag, push `main` and the tag |

On gtlbrunner, `apps/natlas/app.conf` says `SRC=${GTLB_GIT}/natlas.git`, `BUILD="bash scripts/build-in-container.sh ci"`, `OUT=dist/release`, `KEYS="natlas.jks keystore.env"`, and `apps/natlas/compose.yml` builds `natlas-builder:1` from `docker/builder` of `main`. The runner registration `natlas-app` may run only `natlas-builder:*` and mounts `natlas-build-cache:/build-cache` and `keys/natlas:/keys:ro`. **The image tag is the contract:** when `docker/builder/Dockerfile` changes, bump `natlas-builder:N` in `.gitlab-ci.yml` and in gtlbrunner's `apps/natlas/compose.yml`, then `./build.sh -image natlas` there.

The server image (`Dockerfile`) is separate: `docker compose build` on the server, version from `VERSION`.

## 11. Testing and scripts

`go test ./...` (vendored, no network) covers:

- **store:** file untouched on read; comments and unknown fields kept; YAML 1.1 quoting; create, delete, undo, move; nested paths and record collections; CSV byte-identical rows and CRLF; Markdown sections; shared files (`id_field: none`, compact lists, flow rows).
- **query:** operators, date tokens, select order, sorting. **dates:** anniversaries (leap day), cycles, formats, clock times.
- **config:** shipped plugins valid; mistakes collected and reported together; unknown keys rejected; `file:` only on read-only collections; calendar needs `title` and `date`.
- **api:** an end-to-end flow (login, Done, 409, 422, quick add, read-only 403, calendar, cross-origin 403, health move, dashboard), layout (save, fit, reset), read-only demo, quick bookmarks, health upsert and snapshot `charts`, phone `Snapshot` / `Exec`, formatting helpers, path trimming. `NATLAS_BENCH=1` times a 10-year backfill.
- **merge:** copy detection, added records, queued differences, resolve both ways, read-only files skipped.
- **auth:** cookie attributes, tampering, sessions ending on a password change, PBKDF2, lockout, IP bans, device-key sessions, demo login, client-IP extraction.
- **web:** routes, title escaping, `/app/` redirect, every `views/` part included, no listings / source / page parts under `/assets/`, manifest, service worker version and shell list.

The Android build runs the Go tests again and the Kotlin unit tests. UI changes are checked with `scripts/dev.sh` (sample data, `localhost:8080/app`, login `demo` / `demo`) on a phone-sized and a desktop viewport. `scripts/check-public.sh` runs in CI and as a git pre-push hook (with `.private-words`).

## 12. Adding features

| Need | Where |
|---|---|
| A new kind of data | a new `plugins/<id>/plugin.yml` (+ sample in `examples/data/`, + `natlas.example.yml`, + Android `Prefs.ALL` / `Paths`). No code. |
| A new field type | `config.fieldTypes`, `api.convert`, `store` (if stored specially), `js/lists.js` `display()` / `inputType()`, `views/sheet.html` |
| A new data format | a `store.Doc` implementation, `File.Format` / `Plugin.Format` (`format:` key), its collection rules in `config.prepare` |
| A new view (like `calendar`) | `config.Plugin.View` + validation in `prepare`, a `js/<view>.js` part (+ script tag in `index.html` and `sw.js`), `views/plugin/<view>.html` included from `views/plugin.html`, `css/<view>.css` |
| A new screen | `views/<screen>.html` included from `index.html`, routing in `js/core.js` `applyRoute`, a part in `js/` |
| A new condition operator | `query.validOps` and `fieldCond.Match` |
| A new action kind | `config.Action` + validation in `prepare`, `api.action` / `runSet` / `runMove` |
| A new setting | `config.Settings` + `LoadSettings`, `.env.example`, `docker-compose.yml` `environment:`, §4.1 |
| A new build tool | `docker/builder/Dockerfile`, bump the builder tag (§10) |

## 13. Function index

Generated from the source (`go/ast`): every top-level function, method and type with the first sentence of its comment. Tests are left out.

#### `cmd/natlas/commands.go`

commands.go — the one-shot commands: cli, hash-password, health, bans, unban.

| Name | Does |
|---|---|
| `cli` | Answers one request from the phone app's widgets without a server: {"op":"snapshot"} \| {"op":"action",plugin,coll,id,rev,action} \| {"op":"patch… |
| `hashPassword` | Reads one line from stdin and prints the hash for .env. |
| `health` | Is the Docker HEALTHCHECK (the image has no curl). |
| `bans` | Lists the banned IPs; unban <ip\|all> lifts bans. |

#### `cmd/natlas/main.go`

Command natlas serves the Natlas web app.

| Name | Does |
|---|---|
| `main` | Dispatches a subcommand, or runs the server. |
| type `loaded` | Is the result of load: the settings and the app with its plugins. |
| `load` | Reads .env and natlas.yml and every enabled plugin. |

#### `cmd/natlas/middleware.go`

middleware.go — handlers wrapped around every request: headers, access log, panic recovery.

| Name | Does |
|---|---|
| `securityHeaders` | Adds browser protections to every response. |
| type `statusWriter` | Remembers the response status for the access log. |
| `statusWriter.WriteHeader` | Records the status, then writes it. |
| `accessLog` | Records every change (and every failed request) with the client IP - never request bodies, cookies or query strings. |
| `recoverer` | Turns a panic in one request into a logged 500 instead of a dropped connection. |

#### `cmd/natlas/serve.go`

serve.go — the HTTP server: routes, demo login, conflict merging, shutdown.

| Name | Does |
|---|---|
| `serve` | Runs the web server until SIGINT/SIGTERM. |
| `warnMissingFiles` | Logs data files that do not exist. |
| `demoMux` | Serves the API for the demo login: the same plugins over a fresh copy of the sample data (<state dir>/demo), read-only, default layout. |
| `mergeLoop` | Merges Syncthing conflict copies every 15 s (main device only). |
| `writeDemo` | Copies the built-in sample files into dir, never over an existing file. |

#### `examples/examples.go`

Package examples embeds the sample data, so the phone app can offer a demo folder without any download.


#### `internal/api/api.go`

Package api is the HTTP layer: one generic set of routes serves every plugin, driven entirely by plugin.yml.

| Name | Does |
|---|---|
| type `Server` | Holds what every handler needs. |
| `Server.Register` | Adds the API routes to mux. |
| `sameOrigin` | Blocks cross-site writes: a changing request must be JSON (which a foreign page cannot send without CORS) and, when the browser says whe… |
| `writeJSON` | Sends v as JSON with the status. |
| `fail` | Sends {"error": msg} with the status. |
| `failErr` | Maps store and validation errors to HTTP responses. |
| `publicMessage` | Is an error's text with absolute paths cut down to file names. |
| type `badRequest` | Is a client mistake (400) with its message. |
| `badRequest.Error` | Returns the message. |
| `badRequest.Is` | Makes errors.Is(err, errBadRequest) true. |
| `decode` | Reads a JSON body (1 MB max, JSON content type only) into into. |
| `Server.target` | Resolves {plugin}/{coll} from the URL. |
| `writable` | Answers 403 and returns false for a read-only collection. |
| `checkRev` | Finds the record and makes sure the browser saw its latest version. |
| `Server.appInfo` | Handles GET /api/app: title, currency, locale, user, plugin definitions. |
| type `groupOut` | Is one list group in GET /api/p/{p}/{c}. |
| `Server.list` | Handles GET /api/p/{p}/{c}: sorted records, groups, options, note. |
| `options` | Lists dropdown values per select field: the fixed list first, then any other values found in the data (so old values still show correctly). |
| `Server.create` | Handles POST /api/p/{p}/{c}: a new record (optional id and position). |
| `Server.update` | Handles PATCH /api/p/{p}/{c}/{id}: changed fields, checked against rev. |
| `Server.remove` | Handles DELETE /api/p/{p}/{c}/{id}: returns what undo needs. |
| `indexOf` | Is the record's position in its file. |
| `Server.action` | Handles POST …/{id}/do/{action}: runs a plugin action (set or move_to). |
| `findAction` | Returns the collection's action with this id, or nil. |
| `runSet` | Applies an action's `set:` (plus any asked values) to one record. |
| `runMove` | Moves a record into another collection of the same file (Health's "Complete run": planned -> recent), carrying over the fields both share. |
| `askedValues` | Validates the fields an action asked the user for. |
| `Server.bulk` | Handles POST …/bulk: one action or one set of values on many records, one save. |
| `Server.move` | Handles POST …/{id}/move: swaps a record with its neighbour (order: manual). |

#### `internal/api/conflicts.go`

| Name | Does |
|---|---|
| type `fieldDiff` | Is one field that differs between the file and the conflict copy. |
| type `conflictOut` | Is one queued sync conflict for the Today card. |
| `Server.conflicts` | Lists records changed on two devices (GET /api/conflicts), plus conflict copies not merged yet (on a phone they wait for the main device). |
| `Server.resolveConflict` | Applies a choice (POST /api/conflicts/{id} {keep: kept\|other}). |

#### `internal/api/dashboard.go`

| Name | Does |
|---|---|
| type `agendaItem` | Is one entry on the Today agenda. |
| type `actionBrief` | Is an action button on an agenda entry. |
| type `widgetOut` | Is one number on the Today screen. |
| type `statsOut` | Is one plugin's numbers on the Today screen. |
| `Server.dashboard` | Builds the Today screen: agenda sections (same rules as Taskmaster's daily agenda) and a row of numbers per plugin. |
| `daysOr` | Is *d, or 7 when unset. |
| `agendaEntry` | Turns a record matched by an agenda rule into an agenda item. |
| `widgetValue` | Computes a widget (count, sum, monthly…) over the records. |

#### `internal/api/format.go`

| Name | Does |
|---|---|
| `relDays` | Is "today", "tomorrow", "in 3 days", "2 days ago". |
| `fmtDate` | fmtDate: Today / Tomorrow / Mon 20 Oct (this week) / 20 Oct / 20 Oct 2027. |
| `fmtClock` | fmtClock: "20:30" → "8:30 pm". |
| `group` | Adds thousands separators; en-IN groups as 1,23,45,678. |
| `fmtNumber` | Keeps up to 2 decimals. |
| `fmtMoney` | Is the amount with the currency symbol. |
| `fmtStat` | Formats a widget value (money, number, days). |
| `display` | Formats one value of a record (keys may be virtual: birthday.days). |
| `subtitle` | Is the list subtitle of a record. |
| `badgeTone` | Is the colour class of a badge value. |

#### `internal/api/layout.go`

| Name | Does |
|---|---|
| type `Layout` | Is the menu order and the Today screen, edited on #/customize and kept in <state dir>/layout.yml, so the web app and the phone share it. |
| type `Block` | Is one part of the Today screen. |
| `defaultHome` | Is the Today screen without a layout.yml: agenda, then numbers. |
| `Server.layout` | Reads layout.yml and fits it to the loaded plugins: new plugins join the menu at the end, removed ones and broken blocks are dropped. |
| `Server.fit` | Fits a layout to the loaded plugins (defaults: use defaultHome when it has none). |
| `Server.validBlock` | Checks one block and fills its defaults. |
| `Server.plugin` | Returns a loaded plugin by id, or nil. |
| `Server.getLayout` | Handles GET /api/layout. |
| `Server.putLayout` | Saves the whole layout ({"reset":true} goes back to the defaults). |

#### `internal/api/quick.go`

| Name | Does |
|---|---|
| `Server.quickAdd` | Creates a record from one line of text. |
| `fmtAny` | Is a value as text for the quick-add reply. |
| `withDefaults` | Fills fields the quick-add line did not set with their defaults. |
| `parseQuick` | Splits a quick-add line into fields: date words, #tag, !priority, time, link. |
| `clockWords` | Reads "8pm", "20:30" or "8 pm" from the start of words. |
| `matchOption` | Finds the option a short form refers to: "h" / "hi" -> "high". |
| `parseDateWords` | Reads a date from the start of words and says how many words it used (0 = not a date). |
| `weekdayFull` | Reads a weekday name ("mon", "monday"). |
| `monthOf` | Reads a month name ("oct", "october"). |
| `buildDate` | Makes a date; without a year it picks the next occurrence. |

#### `internal/api/snapshot.go`

| Name | Does |
|---|---|
| `Server.call` | Runs one API handler in-process and decodes its JSON answer. |
| `Server.Exec` | Runs one phone-app request: snapshot \| action \| patch \| quick \| merge \| upsert. |
| `Server.undoValues` | Lists the old values of the fields an action changed. |
| `Server.Snapshot` | Builds everything the widgets show. |
| `syncSource` | Names who writes a read-only collection (for the widget footer). |
| `quickHint` | Is the quick-add placeholder of a collection. |
| `whenText` | Is the short date/time label of an agenda item. |
| `listEntry` | Is one collection for the List widget: open groups, ready-made text. |
| `cap50` | Keeps the first 50 items. |
| `containsAny` | Reports whether list holds s. |
| `orDash` | Is s, or "-" when empty. |
| `calendarEntries` | calendarEntries: entries from 7 days back to 62 days ahead. |
| `Server.chartEntry` | chartEntry: the last 35 days of a charts collection, goals and newest values, for the health widgets. |

#### `internal/api/sync.go`

| Name | Does |
|---|---|
| `Server.upsert` | Is the phone's daily sync (natlas cli, op upsert): records matched by the collection's sync.key are updated or added in key order, then the … |
| `applyLatest` | Fills the record's templated fields from the newest non-empty values; a template whose value is missing is skipped. |
| `onlyToday` | Reports whether a latest template uses only {today…} (no data field). |

#### `internal/api/values.go`

| Name | Does |
|---|---|
| type `fieldErrors` | Maps a field key to a message shown next to that input. |
| `fieldErrors.Error` | Lists the field errors. |
| `clean` | Converts raw JSON input into stored values using the field types. |
| `defaultValue` | Is a field's default ("today" resolved), or nil. |
| `convert` | Checks and normalises one value. |
| `contains` | Reports whether list holds s. |
| `resolveSet` | Turns an action's `set:` into stored values ("today" -> a date in the field's own format). |
| `decorate` | Prepares a record for the browser: id, revision, which actions apply, and computed anniversary values ("birthday.days" etc). |
| `addVirtuals` | Adds "<key>.days", "<key>.age", "<key>.next" for anniversary fields. |

#### `internal/auth/auth.go`

Package auth is single-user login: a signed session cookie (HMAC-SHA256, no server-side session store) plus a per-IP lockout after repeated failures.

| Name | Does |
|---|---|
| type `Auth` | Checks credentials and sessions. |
| `New` | Creates the authenticator. |
| `Auth.Bans` | Is the IP ban list (natlas bans / unban use it too). |
| type `session` | Is a decoded, verified session cookie. |
| `Auth.session` | Reads and verifies the session cookie. |
| `Auth.IsDemo` | Is true for a request signed in with the demo login. |
| `Auth.User` | Returns the logged-in username, or "" when the request has no valid session. |
| `Auth.Require` | Wraps a handler so it only runs for logged-in users. |
| `Auth.Guard` | Answers 403 to banned addresses, before anything else runs. |
| `Auth.setSession` | Issues the session cookie: until logout with the device mark, else short. |
| `Auth.shortLife` | Is how long a login without the device key lasts. |
| `Auth.issue` | Writes a signed session cookie. |
| `Auth.deviceMark` | Is the device cookie's value: proof that the device key was entered on this browser (the key itself never leaves the server). |
| `Auth.hasDevice` | Reports whether the browser holds a valid device mark. |
| `Auth.setDevice` | Sets (on) or clears the device-mark cookie. |
| `Auth.blocked` | Answers a banned or locked-out address; true when it did. |
| `Auth.failed` | Records a wrong password or device key and builds the reply text. |
| `Auth.Login` | Handles POST /api/login {username, password}. |
| `Auth.Device` | Handles POST /api/device {key} (logged in): with the right NATLAS_DEVICE_KEY this browser stays signed in until logout, also after the next … |
| `Auth.check` | Compares credentials without leaking which part was wrong or how much of it matched. |
| `Auth.checkDemo` | Is check for the demo login (public credentials, so no failure count). |
| `Auth.Logout` | Handles POST /api/logout. |
| `Auth.Me` | Handles GET /api/me. |
| `Auth.LocalEntry` | Handles GET /local?t=<token> in the phone app: it turns the launch token into a cookie and opens the app (only paths under /app). |
| `Auth.Local` | Reports whether this is the phone app. |
| `sign` | Is base64url(HMAC-SHA256(key, payload)). |
| `Auth.ClientIP` | Is the visitor's address, used for the login lockout and logs. |
| `Auth.recent` | Drops failures older than the lockout window. |
| `Auth.lockedFor` | Is how long ip must still wait (home addresses, 30 min lockout). |
| `Auth.fail` | Counts a failure for ip and returns the failures in the window. |
| `writeJSON` | Sends v as JSON with the status. |

#### `internal/auth/bans.go`

| Name | Does |
|---|---|
| type `Bans` | Is the multi-day IP ban list. |
| type `banFile` | Is bans.yml: bans and recent failures per IP. |
| type `Ban` | Is one banned address. |
| `OpenBans` | Loads (or starts) the ban list at path. |
| `Bannable` | Is false for home, loopback and link-local addresses. |
| `Bans.reload` | Re-reads the file when it changed (an unban from the CLI). |
| `Bans.save` | Writes the file atomically. |
| `recentTimes` | Keeps the times within the failure window. |
| `Bans.Banned` | Reports whether ip is banned, and until when. |
| `Bans.Fail` | Records a failure; returns the attempts left and whether ip is now banned. |
| `Bans.Clear` | Forgets the failures of ip (after a good login). |
| `Bans.List` | Returns the current bans, soonest to end first. |
| `Bans.Unban` | Lifts the ban on ip ("all" lifts every ban); reports how many were lifted. |

#### `internal/auth/password.go`

| Name | Does |
|---|---|
| `HashPassword` | Returns a new salted hash for password. |
| `ValidHash` | Reports whether s looks like a hash made by HashPassword. |
| `parseHash` | Splits pbkdf2-sha256:iter:salt:key. |
| `checkHash` | Verifies password against a hash in constant time. |

#### `internal/config/charts.go`

| Name | Does |
|---|---|
| type `Charts` | Turns a dated list into a dashboard: Today (rings + tiles), Trends, Sleep and Body screens, plus the plain list as "Days". |
| type `Metric` | Is one field shown in charts. |
| `Metric.UnmarshalYAML` | Accepts "steps" as well as {field: steps, goal: 8000}. |
| type `Sleep` | Draws each night from start to end, split into stages. |
| type `Sync` | Marks a list a phone app fills from a device source (Health Connect). |
| type `Latest` | Copies the newest values into a record after every sync, e.g. |
| `TemplateKeys` | Template fields used in a Latest value: "{weight}kg" → [weight]. |
| `Plugin.prepareCharts` | Checks charts: and sync: of one collection. |

#### `internal/config/config.go`

Package config loads everything Natlas is told from outside the code: - Settings: secrets and runtime switches from the environment (.env) - App: n…

| Name | Does |
|---|---|
| type `Settings` | Are read from environment variables. |
| `Settings.Demo` | Is true when the read-only demo login is configured. |
| `LoadSettings` | Reads Settings from the environment and reports every missing required value at once. |
| `envOr` | Is the variable, or fallback when empty. |
| `envInt` | Is the variable as an int, or fallback. |
| `envBool` | Is the variable as a bool (1/true/yes, 0/false/no), or fallback. |
| type `App` | Is natlas.yml. |
| type `PluginRef` | Is one entry under `plugins:` in natlas.yml. |
| `PluginRef.UnmarshalYAML` | Accepts both `- events` and `- {id: events, file: x.md}`. |
| `Load` | Reads natlas.yml and every enabled plugin, then validates the lot. |
| `App.Plugin` | Returns a loaded plugin by id, or nil. |
| `decodeFile` | Strictly decodes a YAML file: unknown keys are errors, so a typo like `requird: true` is caught at startup instead of silently ignored. |
| `decodeBytes` | Decodes YAML strictly (unknown keys are errors). |
| `pluginFile` | Reads plugins/<id>/plugin.yml from disk, or from the copy built into the binary when there is no plugins folder (the phone app). |

#### `internal/config/plugin.go`

| Name | Does |
|---|---|
| type `Plugin` | Is plugins/<id>/plugin.yml: one tab in the app, backed by one data file (read-only collections may read another file, see Collection.File). |
| `Plugin.FileOf` | Is the data file a collection reads: its own `file:` or the plugin's. |
| `Plugin.Format` | Is "csv" or "yaml", decided by the data file's extension. |
| `FormatOf` | Is "csv" for a .csv path, otherwise "yaml". |
| `Plugin.Collection` | Returns a collection by id, or nil. |
| type `Collection` | Is one list (or one single record) inside the plugin's file. |
| type `Field` | Is one column / form input. |
| type `ListSpec` | Controls how the list screen looks. |
| type `Group` | Is a list section such as "Overdue" or "Closed". |
| type `Action` | Is a one-tap button on a record. |
| type `QuickAdd` | Maps the quick-add box onto fields. |
| type `Link` | Is an outside page for a record; {key} in the URL is replaced by that field's value (URL-encoded). |
| type `Summary` | Is the collapsible totals table (Inventory's old pivot). |
| type `Agenda` | Puts matching records on the Today screen. |
| type `Widget` | Is one number on the Today screen's stats row. |
| `Collection.Reindex` | Rebuilds the key -> field lookup (done by Load; tests that build a Collection by hand call it themselves). |
| `Collection.StoresIDs` | Is false for `id_field: none`: ids come from id_from and are never written. |
| `Collection.Field` | Returns a field by key, or nil. |
| `Collection.Lookup` | Adapts the collection's fields for the query package. |
| `Group.Cond` | Is the compiled where: of the group. |
| `Action.Cond` | Is the compiled when: of the action. |
| `Agenda.Cond` | Is the compiled where: of the agenda rule. |
| `Widget.Cond` | Is the compiled where: of the widget. |
| `Plugin.prepare` | Fills defaults, resolves option lists and compiles conditions, collecting every problem so they can be fixed in one go. |
| `resolveOptions` | Turns `options:` into a fixed list and/or "suggest from data". |
| `knownTypes` | Lists the field types for error messages. |
| `humanize` | Turns "next_due_date" into "Next due date". |

#### `internal/dates/dates.go`

Package dates holds every date rule Natlas uses: what "today" is, how date fields are parsed, relative tokens like "today+7", repeat cycles and yea…

| Name | Does |
|---|---|
| `Today` | Returns the current local date at midnight. |
| `Parse` | Reads a date string using layout (ISO when empty). |
| `Format` | Writes t in layout (ISO when empty). |
| `Resolve` | Turns "today", "today+7" or "today-30" into a date. |
| `DaysBetween` | Returns whole days from a to b (negative when b is earlier). |
| `parseCycle` | Splits a rule like "monthly" or "84 days" into (n, unit). |
| `CycleDays` | Is the average length of a cycle in days ("monthly" = 30.44). |
| type `Anniversary` | Describes the next yearly occurrence of a stored date such as a birthday "1995-01-25" (year "0000" means the year is unknown). |
| `NextAnniversary` | Computes the next occurrence of value ("YYYY-MM-DD"). |
| `Clock` | Normalises a time of day to "HH:MM" (24-hour). |

#### `internal/merge/merge.go`

Package merge folds Syncthing conflict copies back into the data files.

| Name | Does |
|---|---|
| type `Conflict` | Is one record that two devices changed differently. |
| type `Report` | Says what one Scan did. |
| `listPath` | Is where pending conflicts are kept (synced, so every device sees them). |
| `Copies` | Lists the conflict copies of every plugin file (merged or not yet). |
| type `pluginFile` | Is one writable data file and the collections stored in it. |
| `pluginFiles` | Groups the writable plugin files (several plugins may share one). |
| `copiesOf` | Finds the Syncthing conflict copies of path. |
| `globEscape` | Escapes glob characters in a file name. |
| `Scan` | Merges every conflict copy it finds. |
| `has` | Reports whether list holds a conflict with id. |
| `mergeCopy` | Merges one conflict copy into its file. |
| `mergeCollection` | Adds records only in the copy and queues records that differ. |
| `conflictID` | Is stable, so scanning the same copy twice never lists it twice. |
| `List` | Returns the records waiting for a decision. |
| `Resolve` | Applies a decision: keep "kept" (the version in the file) or take "other". |
| `takeOther` | Writes the copy's version of a queued record into the file. |
| `load` | Reads the queued conflicts (.natlas/conflicts.yml). |
| `save` | Writes the queued conflicts atomically. |

#### `internal/query/query.go`

Package query evaluates the small filter language used in plugin.yml for list groups, agenda rules, widgets and action conditions: where: { status:…

| Name | Does |
|---|---|
| type `Record` | Is one data row as plain values (string, float64, bool, nil, []any, map[string]any). |
| type `FieldInfo` | Is what the evaluator needs to know about a field. |
| type `Lookup` | Describes a field by key; ok=false for unknown keys. |
| type `Cond` | Is a compiled where-clause. |
| `Compile` | Turns a where mapping into a Cond, rejecting unknown fields and operators so plugin.yml mistakes surface at startup. |
| `compileField` | Compiles the conditions on one field ({op: value} or a bare value). |
| type `allCond` | Matches when every condition matches. |
| `allCond.Match` | Reports whether every condition matches. |
| type `anyCond` | Matches when one condition matches. |
| `anyCond.Match` | Reports whether one condition matches. |
| type `notCond` | Inverts a condition. |
| `notCond.Match` | Reports whether the condition does not match. |
| type `emptyCond` | Matches an empty (or non-empty) field. |
| `emptyCond.Match` | Reports whether the field's emptiness is as wanted. |
| type `fieldCond` | Compares a field with a value by the field's type. |
| `fieldCond.Match` | Applies the operator. |
| `compare` | Returns -1, 0, 1, or 2 when the values cannot be compared. |
| `indexOf` | Is the position of s in list, or -1. |
| `toDate` | Parses a date value with the field's layout. |
| `cmpTime` | Is -1, 0 or 1. |
| `cmpStr` | Compares case-insensitively. |
| `IsEmpty` | Is true for nil, "", and empty lists. |
| `Text` | Renders any value as display text. |
| `ToFloat` | Reads a number from a float, int or numeric string. |
| `Sort` | Orders records by a spec like ["date", "-priority"] ("-" = descending). |

#### `internal/store/csv.go`

| Name | Does |
|---|---|
| type `csvDoc` | Keeps every cell as the raw text from disk. |
| `parseCSV` | Reads a CSV file, keeping each row's raw cells. |
| `csvDoc.col` | Is the column index of name, or -1. |
| `csvDoc.ensureCol` | Adds a column (at the front for the id, else at the end). |
| `csvDoc.toRecord` | Turns a row into a record using the field types. |
| `fromCell` | Gives CSV text its field type: numbers become numbers (empty -> nil), lists split on "\|", everything else stays text. |
| `toCell` | Writes a value as a CSV cell (lists joined with \|). |
| `csvDoc.Items` | Implements Doc. |
| `csvDoc.Top` | Implements Doc (CSV files have no top-level values). |
| `csvDoc.Create` | Implements Doc. |
| `csvDoc.Update` | Implements Doc. |
| `csvDoc.Delete` | Implements Doc. |
| `csvDoc.Move` | Implements Doc. |
| `csvDoc.Save` | Implements Doc. |

#### `internal/store/lock_other.go`

| Name | Does |
|---|---|
| `lockPath` | Is a no-op where flock does not exist (Windows development builds). |

#### `internal/store/lock_unix.go`

| Name | Does |
|---|---|
| `lockPath` | Takes an exclusive flock on <dir>/<hash of path>.lock (waits for the other process). |

#### `internal/store/markdown.go`

| Name | Does |
|---|---|
| type `markdownDoc` | Treats a Markdown note as records, so the same note can be edited in Natlas and in Obsidian: --- front matter -> collection path "front… |
| type `mdSection` | Is one "## " section: its raw text, title and body. |
| `parseMarkdown` | Splits a note into front matter, intro and sections. |
| `mdSection.record` | Is the section as {title, body}. |
| `markdownDoc.frontRecord` | Is the front matter plus the intro. |
| `markdownDoc.check` | Refuses collections other than sections (list) and front (record). |
| `markdownDoc.Items` | Implements Doc. |
| `markdownDoc.Top` | Implements Doc: a front-matter value. |
| `oneLine` | Keeps a section title on its heading line. |
| `text` | Is a value as a string. |
| `markdownDoc.Create` | Implements Doc: a new section (at = position, -1 = new_at). |
| `markdownDoc.Update` | Implements Doc. |
| `markdownDoc.Delete` | Implements Doc. |
| `markdownDoc.Move` | Implements Doc: swap a section with its neighbour. |
| `markdownDoc.Save` | Implements Doc. |

#### `internal/store/store.go`

Package store reads and writes the plugin data files (YAML or CSV).

| Name | Does |
|---|---|
| type `Record` | Is one data row as plain values. |
| type `Item` | Is a record plus its identity. |
| `Index` | Is the record's position in the file (used to undo a delete in place). |
| type `Doc` | Is one loaded data file. |
| type `File` | Is one data file plus a lock that serialises Natlas's own writes. |
| `For` | Returns the shared File for a plugin's own data file (one lock per path). |
| `ForCollection` | Returns the file a collection reads (its own `file:` or the plugin's). |
| `forPath` | Returns the shared File for a path (one lock per path). |
| `File.Read` | Loads the file. |
| `File.View` | Is Read for callers that only look (lists, dashboard, snapshot): an unchanged file is parsed once and shared. |
| `File.Change` | Loads the file, runs fn under the lock, and saves if fn succeeds. |
| `Rev` | Hashes a record's content. |
| `Slug` | Makes a URL-safe id fragment: "WBM26 wipro marathon" -> "wbm26-wipro-marathon". |
| `Unique` | Returns base, or base-2, base-3 ... |
| `assignIDs` | Gives every item an id: the stored one, or a deterministic in-memory one from the collection's id_from field. |
| `find` | Returns the item with id, or ErrNotFound. |
| `ParseFile` | Reads any data file in the given format (yaml, csv, markdown) without going through a plugin; used to read Syncthing conflict copies. |
| `WriteAtomic` | Is writeAtomic for other packages (conflict lists, snapshots). |
| `writeAtomic` | Replaces path with data via a temp file in the same folder, keeping the original file's permissions. |

#### `internal/store/upsert.go`

| Name | Does |
|---|---|
| type `Upserter` | Is a Doc that can add-or-update many records by a key field at once (the phone's daily health sync). |
| `yamlDoc.Upsert` | Sets the given fields on the record whose `key` matches, or adds a new record in key order (the list is assumed sorted by key, as the sync w… |
| `setFields` | Writes fields whose value differs; reports whether anything changed. |
| `indexOf` | Is the position of n in list, or -1. |

#### `internal/store/yaml.go`

| Name | Does |
|---|---|
| type `yamlDoc` | Edits the YAML node tree in place, so everything Natlas does not touch - other records, unknown keys, comments, key order - survives a save. |
| `parseYAML` | Parses a file into its node tree and remembers its layout. |
| `mapIndex` | Is the index of key's key node in a mapping, or -1. |
| `mapGet` | Is the value node of key in a mapping, or nil. |
| `yamlDoc.at` | Follows a dotted path ("runs.planned"). |
| `yamlDoc.Items` | Implements Doc. |
| `yamlDoc.Top` | Implements Doc. |
| `toRecord` | Turns a mapping node into a record. |
| `matchStyle` | Writes a new record the way its neighbours are written: one-line `{ name: …, tags: [ … ] }` rows stay one-line (files other apps kee… |
| `quoteFlow` | Writes a one-line row's values like the hand-kept rows: lists inline, text "double-quoted" (dates keep their 'single' quotes). |
| `toValue` | Converts a node to a plain value. |
| `yamlDoc.Create` | Implements Doc. |
| `yamlDoc.Update` | Implements Doc. |
| `yamlDoc.Delete` | Implements Doc. |
| `yamlDoc.Move` | Implements Doc. |
| `yamlDoc.Save` | Implements Doc: "updated: <stamp>" + blank line (when the file had one) + the YAML body, two-space indented. |
| `yamlDoc.restyle` | Keeps the file's own layout where yaml.v3 has no option for it, so saving one record does not rewrite every line of a hand-kept file. |
| `strNode` | Is a plain string scalar node. |
| `valueNode` | Builds a node for a plain value. |
| `yaml11Ambiguous` | Reports whether PyYAML would read s as something other than a string. |

#### `plugins/plugins.go`

Package plugins embeds the shipped plugin folders, so a binary without a plugins/ folder next to it (the Android app) still has every plugin.


#### `web/web.go`

Package web serves the browser app: index.html, CSS, JS, icons, the service worker and a manifest generated from the enabled plugins.

| Name | Does |
|---|---|
| `Handler` | Returns the static-site handler. |
| `render` | Reads a page and replaces each include line with that file (nested up to 3 deep). |
| type `Demo` | Is the read-only demo login shown on the sign-in screen (empty = none). |
| `Demo.html` | Is the sign-in hint; tapping it fills the form. |
| `hashFS` | Fingerprints every file so a new release changes the version. |
| `buildManifest` | Makes the PWA manifest, with a long-press shortcut per plugin. |


## Changelog

- **1.0.3 (2026-11-11).** APK builds only through gtlbrunner (`docker/builder/`, `scripts/build-in-container.sh`, `upload-release.sh`, `check-version.sh`, new `.gitlab-ci.yml`); `android/build.sh` + its env files removed. Web app split into `views/` (server-side includes), `js/` parts and `css/`. `cmd/natlas` split into four files; every Go function commented; this file covers every file and function. `.env.example` rewritten.
- **1.0.2 (2026-10-30).** Demo login moved to the sign-in form; `scripts/release.sh`.
- **1.0.1 (2026-10-30).** `plugins/bookmarks` + `plugins/locations` (`id_field: none`, `link: {url: "{url}"}`, `quick_add.url`, list-field tag options/filters); yaml `restyle` (compact lists, padded flow rows); `/api/layout` + `#/customize` + Today blocks; demo login (`x` sessions, demo mux, `ReadOnly`); landing page `{{DEMO}}`; `.gitlab-ci.yml` (test, apk on the `natlas-apk` runner, release vi…
- **1.0.0 (2026-10-20).** Version reset (old 2.0.0–3.1.1 → 0.2.0–0.6.1; Android versionCode epoch 100000). `auth/bans.go` multi-day IP bans with `Guard`, `natlas bans` / `unban`; session kinds `s` (12 h) / `d` (until logout, device key, `POST/DELETE /api/device`); `NATLAS_CONFIG_DIR` mounted at `/config` (natlas.yml + bans.yml); Android `AppConfig` (config folder: settings.json, natlas.yml, plu…
- **0.6.0 (2026-10-20).** Health from the phone: `charts:` and `sync:` collection options, `store` upsert, cli `upsert`, snapshot `charts`, parse cache for reads, Health → Daily (Today / Trends / Sleep / Body / Days), Android Health Connect reader + morning job + history import, four health widgets.
- **0.5.0 (2026-10-19).** Android app and sync-conflict merge: phone mode (`NATLAS_MODE=local`, token cookie, embedded plugins and sample data), `natlas cli` + `api.Exec` / `Snapshot` / `format.go`, `internal/merge` + `/api/conflicts` + Today card and dialog + `NATLAS_MAIN`, `store.LockDir` flock between natlas processes, `#/phone` settings page and deep links in the web app, `android/` (Kotlin…
- **0.4.0 (2026-10-19).** Architecture page: `store/markdown.go` (`format: markdown`), `view: document`, `markdown` field type, vendored marked + mermaid (lazy), `plugins/architecture`, `examples/data/homelab.md`, store + API + config tests.
- **0.3.1 (2026-10-18).** Calendar replaces Birthdays: `view: calendar`, `week_start`, read-only collection `file:`, field type `time`, `quick_add.time`, `link`.
- **0.3.0 (2026-10-07).** Production hardening: about page at `/`, app at `/app`, PBKDF2 hashes, `__Host-` cookie, safe client IP, security headers, `/healthz`.
- **0.2.0 (2026-10-05).** Rewrite in Go: YAML-only plugins, list + edit sheet, Today screen, nss theme, PWA, record revisions.
