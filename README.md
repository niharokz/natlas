# Natlas

**A small, self-hosted app for your personal lists, backed by plain YAML and CSV files.**

Natlas gives you a fast, phone-friendly web app over files you already keep, for example in an Obsidian vault. There is no database. Each tab reads and writes one human-readable file, so your data stays greppable, diffable, and editable by hand or by your own scripts.

Every tab is a plugin, and a plugin is **one YAML file**: fields, dropdowns, list layout, one-tap actions and dashboard numbers. Adding a new kind of data needs no code.

- **Single Go binary.** About 8 MB, starts instantly, uses about 10 MB of RAM. One dependency (YAML), vendored.
- **Installable app (PWA).** Bottom navigation on phones, swipe gestures, offline read-only mode.
- **Plays well with other writers.** It edits only the record you changed, keeps comments and unknown fields, never re-sorts your file, and refuses to overwrite a record that changed underneath you.
- **Android app with widgets.** The same app offline on your phone, over the folder Syncthing keeps in sync, with 9 home-screen widgets and fingerprint lock. See [android/README.md](android/README.md).
- **No build step for the UI.** Alpine.js and the [nss](https://gitlab.com/niharokz/nss) stylesheet are vendored. Light and dark themes follow your system.

---

## What you can do

| | |
|---|---|
| **Today screen** | Overdue, today and next 7 days from every plugin in one list, with a Done button on each item, plus a row of numbers per plugin. |
| **One-tap actions** | Buttons such as Done, Tomorrow, Cancel or Completed, defined per plugin. |
| **Change in place** | Tap a badge (`open`, `high`, `Good`…) to pick a new value without opening the form. |
| **Swipe** | Swipe right runs the main action; swipe left opens the form. |
| **Bulk edit** | Long-press (or tick, on desktop) several rows, then apply an action or set a field on all of them. |
| **Undo** | Every change shows an Undo button for a few seconds. |
| **Quick add** | `Call bank tomorrow #todo !high` sets the title, date, tag and priority in one line; `Dinner fri 8pm` also sets a time. |
| **Document view** | A Markdown note as a handbook: contents, numbered sections, diagrams, per-section editing. |
| **Calendar view** | Month, week and schedule views for any plugin with dated entries, with per-calendar show / hide and category colours. |
| **Edit form** | A full-screen sheet on phones or a side panel on desktop, with per-field validation messages. |
| **Search, filters, groups** | Lists are split into sections (Overdue / Today / Later / Closed…). Filters and search are one tap away. |
| **Summary** | Totals grouped by any field (for example, inventory value per storage place). |
| **Charts** | A dated list can become a dashboard: progress rings, tiles with the change against your 30-day average, trends with goal lines, a sleep timeline, body measurements. Tap any bar or line to read it. |

## Included plugins

| Plugin | File | Highlights |
|---|---|---|
| **Events** | `event.md` | Todos and reminders, grouped Overdue / Today / Next 7 days / Later / Closed. Done, Tomorrow and Reopen actions, smart quick add. |
| **Subscriptions** | `subscription.md` | Recurring bills, "due in 7 days", monthly spend across any cycle (`monthly`, `yearly`, `84 days`…). |
| **Calendar** | `calendar-local.md` + `calendar.md` | Month, week and schedule views like Google Calendar. Tap a day to add; edit, move or delete your own entries; show or hide each calendar. Entries synced from Google into `calendar.md` (for example by a nightly job) appear alongside, read-only, with an "Open in Google Calendar" link. Birthdays and holidays from it land on the Today screen. |
| **Health** | `health.md` | **Daily** health data from the phone (steps, sleep and its stages, heart rate, HRV, SpO2, weight, body fat, workouts, water, food…) filled every morning by the Android app from Health Connect / Samsung Health, shown as Today rings and tiles, Trends (7 days to all time, weekly and monthly averages), a Sleep timeline and Body charts. Plus planned races (with a "Completed" action that asks for time and cost and moves the race to history), race history, gym routine (manual order), and a profile record kept up to date with your latest weight. |
| **Architecture** | `homelab.md` | One Markdown note shown as a handbook: numbered sections with a contents sidebar, tables, callouts (`> [!warning]`) and Mermaid diagrams. Edit a section (with preview), add, move or delete sections; the note stays a normal Markdown file you can also edit in Obsidian, and saving one section never overwrites the others. |
| **Inventory** | `inventory.csv` | Belongings, condition and storage, with value and weight totals. |

Turn plugins on and off, and choose their menu order, in `natlas.yml`.

---

## Quick start

### Try it locally (Go 1.24+)

```bash
sh scripts/dev.sh        # open http://localhost:8080/app and log in with demo / demo
```

This runs against a temporary copy of `examples/data/`, so nothing you click is permanent.

### Run with Docker

```bash
git clone https://gitlab.com/niharokz/natlas.git && cd natlas
cp .env.example .env              # login, secret key, data folder, timezone
cp natlas.example.yml natlas.yml  # app title, currency, enabled plugins
docker compose build

# password hash -> NATLAS_PASSWORD_HASH in .env (typed password stays out of shell history)
read -rs PW && printf '%s' "$PW" | docker run --rm -i natlas:local hash-password
openssl rand -hex 32              # -> NATLAS_SECRET_KEY in .env

docker compose run --rm natlas check   # every config mistake, with file and key
docker compose up -d
```

Natlas serves a public about page at `/` and the app at `/app`. Open `/app`, sign in, then use your browser's **Install** / **Add to Home Screen**.

The container joins an existing Docker network (`NATLAS_NETWORK`, default `homelab`) and listens on port **8080** without publishing it. Point your reverse proxy at it. For example, with Caddy:

```caddyfile
natlas.example.com {
    reverse_proxy natlas:8080
}
```

> **HTTPS is required** because the login cookie is `Secure`. For a plain-http test, set `NATLAS_COOKIE_SECURE=false` and add `ports: ["8080:8080"]`.

Updating later: `git pull && docker compose up -d --build`. After editing a `plugin.yml` or `natlas.yml`, `docker compose restart natlas` is enough.

Commands built into the binary: `natlas check`, `natlas hash-password`, `natlas health` (used by the Docker health check, which calls the unauthenticated `GET /healthz`) and `natlas version`.

### Android app

```sh
cp android/.env.example android/.env && ./android/build.sh keystore   # once
./android/build.sh                                                     # → android/dist/natlas-<version>.apk
```

The app runs this same engine on the phone in **phone mode** (`NATLAS_MODE=local`): bound to `127.0.0.1`, unlocked by a per-launch token instead of a password, plugins and sample data built in. Everything else - widgets, lock, permissions - is in [android/README.md](android/README.md).

---

## Configuration

### `.env` (secrets and machine settings, never committed)

`docker-compose.yml` passes only the variables below into the container, so `.env` may be a file shared with other services.

| Variable | Purpose |
|---|---|
| `NATLAS_USERNAME` | The single login name. |
| `NATLAS_PASSWORD_HASH` | Password hash from `natlas hash-password` (PBKDF2-SHA256). Or set a plain `NATLAS_PASSWORD` instead. |
| `NATLAS_SECRET_KEY` | Signs the session cookie. At least 32 characters (`openssl rand -hex 32`). Changing it, or the password, signs every session out. |
| `NATLAS_DATA_DIR` | Host folder with your data files. It is mounted at `/data`. |
| `TZ` | Timezone used for "today", "overdue" and calendar dates. |
| `PUID`, `PGID` | The container runs as this user, so saved files belong to you. |
| `NATLAS_NETWORK` | Existing Docker network shared with your reverse proxy. |
| `NATLAS_COOKIE_SECURE` | `false` only for plain-http testing. |
| `NATLAS_MAIN` | Default `true`: merge Syncthing conflict copies record by record (see below). Set `false` if another device does it. |
| `NATLAS_CONFIG_DIR` | Host folder mounted at `/config`: holds `natlas.yml` and `bans.yml`. Must be writable by `PUID`. Default `./config`. |
| `NATLAS_DEVICE_KEY` | Optional, 32+ characters (`openssl rand -hex 32`). A browser given this key (app → **This device**) stays signed in until you log out; others are signed out after `NATLAS_SESSION_HOURS`. A new key signs every remembered browser out. |
| `NATLAS_SESSION_HOURS` | Life of a login without the device key. Default `12`. |
| `NATLAS_MAX_FAILURES`, `NATLAS_BAN_DAYS` | Failed logins (or device keys) within a day before a public IP is banned, and for how many days. Default `5` and `7`. Home addresses only get a 30-minute lockout. |
| `NATLAS_TRUST_PROXY` | Default `true`: behind a proxy, reads the visitor IP (for the lockout and logs) from `CF-Connecting-IP`, `X-Real-IP`, or the **last** `X-Forwarded-For` entry. Set `false` if nothing sits in front of Natlas. |

### `natlas.yml` (app settings, no secrets)

```yaml
title: Natlas
currency: "₹"
locale: en-IN
data_dir: /data
plugins:                 # enabled plugins, in menu order
  - events
  - subscriptions
  - { id: inventory, file: stuff.csv, title: Stuff }   # per-plugin overrides
lists:                   # shared dropdown lists, used as options: "@rooms"
  rooms: [Kitchen, Bedroom, Garage]
```

---

## Writing a plugin

Create `plugins/<id>/plugin.yml`, add `<id>` to `natlas.yml`, and restart. Here is a complete plugin:

```yaml
title: Books
icon: box                       # home calendar wallet gift heart box, or an emoji
file: books.md                  # inside data_dir; .csv files work too

collections:
  - id: books
    path: books                 # where the list lives in the YAML file
    fields:
      - { key: title, type: text, required: true }
      - { key: author, type: select, options: data }        # suggests values already in the file
      - { key: status, type: select, options: [to read, reading, read], default: to read }
      - { key: finished, type: date }
      - { key: rating, type: number }
    list:
      title: title
      subtitle: [author, finished]
      badges: [status]          # tap to change in place
      search: [title, author]
      filters: [author, status]
      groups:
        - { label: Reading, where: { status: reading } }
        - { label: To read, where: { status: to read } }
        - { label: Read, where: { status: read }, sort: [-finished], collapsed: true }
    actions:
      - { id: finish, label: Finished, icon: "✓", primary: true,
          set: { status: read, finished: today }, when: { status: reading } }
    quick_add: { title: title }
    widgets:
      - { label: Reading, count: { status: reading } }
      - { label: Read this year, count: { status: read, finished: { gte: today-365 } } }
```

### Field types

`text` · `textarea` · `number` (with optional `unit`) · `money` · `date` (with optional `format` in Go layout, for example `"02 Jan 2006"`) · `select` (`options: [..]`, `data`, or `"@list"`; add `allow_new: true` to allow typing new values) · `bool` · `list` (one item per line) · `url` · `time` (a time of day, stored as `HH:MM`; `6:30pm` is accepted) · `markdown` (multi-line Markdown, edited with a Write / Preview switch) · `anniversary` (a yearly date such as a birthday).

Other field options are `required`, `default` (a value, or `today` / `today+7` for dates), `readonly`, `hidden`, `placeholder`, `help`, and `tones` (badge colours, for example `{ pending: warn }`).

### Conditions (`where`, `when`, `count`)

```yaml
{ status: open }                                  # equals
{ status: [open, pending] }                       # one of
{ date: { lt: today } }                           # eq ne in nin lt lte gt gte empty contains
{ date: { gt: today, lte: today+7 } }             # all must hold
{ any: [ { status: pending }, { date: { lt: today } } ] }   # also: all, not
```

Dates compare as dates, numbers as numbers, and select fields by the order of their options (so `priority: { lte: medium }` means high or medium). Anniversary fields add `<key>.days`, `<key>.age` and `<key>.next` for use in conditions and lists.

### Collection options

| Key | Meaning |
|---|---|
| `kind: record` | A single mapping edited as one form (for example, a profile) instead of a list. |
| `readonly: true` | View only: no add, edit or delete. |
| `file` | Read-only collections only: read another data file than the plugin's (for example one a sync job rewrites every night). |
| `link` | `{ label, url }` - a button on each record opening an outside page; `{key}` in the URL is replaced by that field. |
| `order: manual` | Keep the file order and offer move up/down. Otherwise lists are sorted on screen, never in the file. |
| `new_at: start` | Insert new records at the top of the file. |
| `id_from` | The field whose slug becomes a new record's id (default: `title` or `name`). |
| `note` | Text under the title. `{key}` inserts a top-level value from the file, for example `{last_sync}`. |
| `agenda` | Puts matching records on the Today screen: `{ section: overdue \| today \| upcoming, where, date }`. |
| `widgets` | Today-screen numbers: `{ label, count: <where> }` or `{ label, sum: field, where, per_month_by: cycle-field, format: money }`. |
| `summary` | `{ group_by: [...], sum: [...] }` adds a totals table to the list. |
| `actions` | `{ id, label, icon, primary, set, when, confirm }`, or `{ move_to: other-collection, ask: [fields] }` to move a record between lists in the same file. |
| `quick_add` | `{ title, date, tag, priority, time, hint }` - which fields the one-line quick add fills. |
| `charts` | Turns a dated list into a dashboard with Today / Trends / Sleep / Body / Days tabs: `{ date, goals, rings, tiles, trends, sleep, body }` (see below). |
| `sync` | `{ source: health_connect, key: date, latest: { collection, set } }` - the Android app fills this list from Health Connect; `latest` copies the newest values into a record (`weight: "{weight}kg"`, `last_updated: "{today\|02 Jan 2006}"`). |

#### Charts

```yaml
charts:
  date: date                 # the day field
  goals: profile             # a record whose goal_<field> values override the goals below
  rings:  [{ field: steps, goal: 8000 }, { field: sleep_h, goal: 7.5 }]   # up to 3
  tiles:  [km, { field: hr_rest, lower: true }]                          # lower: lower is better
  trends: [{ field: steps, goal: 8000, total: true }, { field: hrv, chart: line }]
  sleep:  { hours: sleep_h, start: sleep_start, end: sleep_end, goal: 7.5, stages: [deep_h, rem_h, light_h, awake_h] }
  body:   [{ field: weight, chart: line, decimals: 1 }]
```

A metric is a number field, written as its name or `{ field, label, unit, goal, chart: bar | line, total, lower, decimals }`. Units come from the field (`unit: h` shows hours as `7h 12m`). Trends group by day up to a month, by week up to four months, by month beyond, and compare with the period before.

### Plugin options

| Key | Meaning |
|---|---|
| `file` | The data file (relative to `data_dir`). `.csv` for one CSV list, anything else is YAML (unless `format: markdown`). |
| `format: markdown` | The file is a Markdown note: `path: sections` (a list; every `## Heading` is a record with `title` and `body`) and `path: front` (a record: front-matter keys plus `intro`, the text before the first section). Sections and the front matter Natlas did not edit are written back byte for byte; a `## ` inside a code block is not a heading. |
| `view: document` | Show a Markdown note as a handbook (needs `format: markdown`): contents sidebar, numbered sections, GitHub-style Markdown, callouts (`> [!note]`, `[!tip]`, `[!warning]`, `[!danger]`, `[!info]`), Mermaid diagrams (loaded only when a section has one). |
| `view: calendar` | Show all collections merged as a month / week / schedule calendar instead of lists. Each collection needs `title` and `date` fields; `time`, `end`, `category` (colour), `calendar` and `location` are used when present. New entries go to the first writable collection. |
| `week_start` | `monday` (default) or `sunday`, for the calendar view. |

---

## How Natlas treats your files

- **Only the edited record changes.** Other records, unknown fields, comments and key order are left as they were. CSV rows you did not touch stay byte-for-byte identical.
- **The file is never re-sorted.** Sorting and grouping happen on screen only, so a script that keeps its own order is never undone.
- **Records without an `id`** get a stable id in memory. It is written to the file only when that record is edited. Opening a tab never writes.
- **Conflicts are refused, not overwritten.** Every record has a revision. If another program changed the record after you opened it, the save is refused with a "changed elsewhere, reload" message.
- **Writes are atomic** (temporary file, then rename), so readers never see a half-written file.
- **YAML 1.1 safe.** Values that tools like PyYAML would misread (`'65:30'`, `'2026-10-05'`, `'yes'`) are quoted, just as `yaml.safe_dump` does.
- **Sync conflicts are merged, record by record.** When Syncthing leaves `event.sync-conflict-…md` next to `event.md`, the main device (`NATLAS_MAIN`) adds back records that exist only in the copy and deletes the copy. A record changed on both sides keeps the file's version; the other one waits on the Today screen ("1 record was changed on two devices") until you pick one.
- A YAML file that starts with an `updated: <timestamp>` line gets a fresh stamp on every save.

---

## Project layout

```
cmd/natlas/        entry point (serve, check, hash-password, health, version, cli), middleware
internal/config/   .env, natlas.yml and plugin.yml loading and validation
internal/store/    YAML and CSV reading and writing
internal/query/    the where / sort language
internal/api/      HTTP API, dashboard, quick-add parser
internal/auth/     login, signed session cookie, lockout
internal/dates/    today, date formats, cycles, anniversaries
internal/merge/    record-by-record merge of Syncthing conflict copies
web/               landing page (/), the app (/app: index.html, app.js, natlas.css), service worker, vendored nss and Alpine
plugins/           one folder per plugin, each holding a plugin.yml
examples/data/     sample data for trying it out (also built into the binary)
android/           the Android app: Kotlin shell, widgets, Docker build (android/build.sh)
scripts/           dev.sh (local run), check-public.sh (pre-push secret check)
```

Run the tests with `go test ./...` (the `vendor/` folder is used automatically; no network needed).

## Security notes

- **Login:** one user. The password can be stored as a PBKDF2-SHA256 hash and is compared in constant time; every failure is delayed. Five wrong logins (or device keys) in a day **ban a public IP for 7 days**: every request from it gets 403, and the ban survives restarts (`bans.yml` in the config folder). Only you lift it: `docker exec natlas /natlas unban <ip>` (`bans` lists them). Home/LAN, loopback and Docker addresses are never banned, only locked out for 30 minutes, so a wrong proxy header can never ban the proxy itself.
- **Sessions:** HMAC-SHA256 signed cookie, `HttpOnly`, `Secure`, `SameSite=Lax`, named `__Host-natlas` over HTTPS. A login lasts 12 hours (`NATLAS_SESSION_HOURS`). Enter the device key (`NATLAS_DEVICE_KEY`) under **This device** and that browser stays signed in until you log out, also after the next login (an `HttpOnly` device cookie proves the key was given; the key itself is never stored in the browser). The signing keys are derived from the secret key, the password and the device key, so changing any of them signs the matching sessions out.
- **Requests:** every change must be same-origin JSON, which blocks cross-site forms and requests. Bodies are capped at 1 MB.
- **Browser:** strict Content-Security-Policy (`'unsafe-eval'` is needed by Alpine.js), HSTS, no framing, `nosniff`, a restrictive Permissions-Policy, and no third-party scripts, fonts or analytics.
- **Container:** runs as your user, read-only root filesystem, all Linux capabilities dropped, `no-new-privileges`; it can only write to the data folder.
- **Logs:** every change and every failed request is logged with the client IP, never with bodies, cookies or passwords.
- **Exposure:** the about page (`/`), `/healthz` and the web manifest are public; everything else needs a session. If you put Natlas on the internet, an extra layer such as a VPN or Cloudflare Access is still a good idea.
- **Publishing your fork:** `scripts/check-public.sh` works as a git pre-push hook. It blocks pushing `.env`, `natlas.yml`, `HOMELAB.md`, anything that looks like a real secret, or any word you list in `.private-words`.

Found a security problem? Please report it privately; see `SECURITY.md`.

## License

MIT. Vendored: Alpine.js (MIT), nss (MIT), go-yaml v3 (MIT and Apache-2.0).
