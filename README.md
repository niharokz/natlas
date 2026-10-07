# Natlas

**A small, self-hosted app for your personal lists, backed by plain YAML and CSV files.**

Natlas gives you a fast, phone-friendly web app over files you already keep, for example in an Obsidian vault. There is no database. Each tab reads and writes one human-readable file, so your data stays greppable, diffable, and editable by hand or by your own scripts.

Every tab is a plugin, and a plugin is **one YAML file**: fields, dropdowns, list layout, one-tap actions and dashboard numbers. Adding a new kind of data needs no code.

- **Single Go binary.** About 8 MB, starts instantly, uses about 10 MB of RAM. One dependency (YAML), vendored.
- **Installable app (PWA).** Bottom navigation on phones, swipe gestures, offline read-only mode.
- **Plays well with other writers.** It edits only the record you changed, keeps comments and unknown fields, never re-sorts your file, and refuses to overwrite a record that changed underneath you.
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
| **Quick add** | `Call bank tomorrow #todo !high` sets the title, date, tag and priority in one line. |
| **Edit form** | A full-screen sheet on phones or a side panel on desktop, with per-field validation messages. |
| **Search, filters, groups** | Lists are split into sections (Overdue / Today / Later / Closed…). Filters and search are one tap away. |
| **Summary** | Totals grouped by any field (for example, inventory value per storage place). |

## Included plugins

| Plugin | File | Highlights |
|---|---|---|
| **Events** | `event.md` | Todos and reminders, grouped Overdue / Today / Next 7 days / Later / Closed. Done, Tomorrow and Reopen actions, smart quick add. |
| **Subscriptions** | `subscription.md` | Recurring bills, "due in 7 days", monthly spend across any cycle (`monthly`, `yearly`, `84 days`…). |
| **Birthdays** | `birthdays.md` | Read-only, sorted by next birthday, shows the age they turn. |
| **Health** | `health.md` | Planned races (with a "Completed" action that asks for time and cost and moves the race to history), race history, gym routine (manual order), and a profile record. |
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
| `TZ` | Timezone used for "today", "overdue" and birthdays. |
| `PUID`, `PGID` | The container runs as this user, so saved files belong to you. |
| `NATLAS_NETWORK` | Existing Docker network shared with your reverse proxy. |
| `NATLAS_COOKIE_SECURE` | `false` only for plain-http testing. |
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

`text` · `textarea` · `number` (with optional `unit`) · `money` · `date` (with optional `format` in Go layout, for example `"02 Jan 2006"`) · `select` (`options: [..]`, `data`, or `"@list"`; add `allow_new: true` to allow typing new values) · `bool` · `list` (one item per line) · `url` · `anniversary` (a yearly date such as a birthday).

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
| `order: manual` | Keep the file order and offer move up/down. Otherwise lists are sorted on screen, never in the file. |
| `new_at: start` | Insert new records at the top of the file. |
| `id_from` | The field whose slug becomes a new record's id (default: `title` or `name`). |
| `note` | Text under the title. `{key}` inserts a top-level value from the file, for example `{last_sync}`. |
| `agenda` | Puts matching records on the Today screen: `{ section: overdue \| today \| upcoming, where, date }`. |
| `widgets` | Today-screen numbers: `{ label, count: <where> }` or `{ label, sum: field, where, per_month_by: cycle-field, format: money }`. |
| `summary` | `{ group_by: [...], sum: [...] }` adds a totals table to the list. |
| `actions` | `{ id, label, icon, primary, set, when, confirm }`, or `{ move_to: other-collection, ask: [fields] }` to move a record between lists in the same file. |

---

## How Natlas treats your files

- **Only the edited record changes.** Other records, unknown fields, comments and key order are left as they were. CSV rows you did not touch stay byte-for-byte identical.
- **The file is never re-sorted.** Sorting and grouping happen on screen only, so a script that keeps its own order is never undone.
- **Records without an `id`** get a stable id in memory. It is written to the file only when that record is edited. Opening a tab never writes.
- **Conflicts are refused, not overwritten.** Every record has a revision. If another program changed the record after you opened it, the save is refused with a "changed elsewhere, reload" message.
- **Writes are atomic** (temporary file, then rename), so readers never see a half-written file.
- **YAML 1.1 safe.** Values that tools like PyYAML would misread (`'65:30'`, `'2026-10-05'`, `'yes'`) are quoted, just as `yaml.safe_dump` does.
- A YAML file that starts with an `updated: <timestamp>` line gets a fresh stamp on every save.

---

## Project layout

```
cmd/natlas/        entry point (serve, check, hash-password, health, version), middleware
internal/config/   .env, natlas.yml and plugin.yml loading and validation
internal/store/    YAML and CSV reading and writing
internal/query/    the where / sort language
internal/api/      HTTP API, dashboard, quick-add parser
internal/auth/     login, signed session cookie, lockout
internal/dates/    today, date formats, cycles, anniversaries
web/               landing page (/), the app (/app: index.html, app.js, natlas.css), service worker, vendored nss and Alpine
plugins/           one folder per plugin, each holding a plugin.yml
examples/data/     sample data for trying it out
scripts/           dev.sh (local run), check-public.sh (pre-push secret check)
```

Run the tests with `go test ./...` (the `vendor/` folder is used automatically; no network needed).

## Security notes

- **Login:** one user. The password can be stored as a PBKDF2-SHA256 hash and is compared in constant time. Five failures from one IP lock that IP out for 30 minutes, and every failure is delayed.
- **Sessions:** HMAC-SHA256 signed cookie, `HttpOnly`, `Secure`, `SameSite=Lax`, named `__Host-natlas` over HTTPS. The signing key is derived from the secret key *and* the password, so changing either signs every session out.
- **Requests:** every change must be same-origin JSON, which blocks cross-site forms and requests. Bodies are capped at 1 MB.
- **Browser:** strict Content-Security-Policy (`'unsafe-eval'` is needed by Alpine.js), HSTS, no framing, `nosniff`, a restrictive Permissions-Policy, and no third-party scripts, fonts or analytics.
- **Container:** runs as your user, read-only root filesystem, all Linux capabilities dropped, `no-new-privileges`; it can only write to the data folder.
- **Logs:** every change and every failed request is logged with the client IP, never with bodies, cookies or passwords.
- **Exposure:** the about page (`/`), `/healthz` and the web manifest are public; everything else needs a session. If you put Natlas on the internet, an extra layer such as a VPN or Cloudflare Access is still a good idea.
- **Publishing your fork:** `scripts/check-public.sh` works as a git pre-push hook. It blocks pushing `.env`, `natlas.yml`, `HOMELAB.md`, anything that looks like a real secret, or any word you list in `.private-words`.

Found a security problem? Please report it privately; see `SECURITY.md`.

## License

MIT. Vendored: Alpine.js (MIT), nss (MIT), go-yaml v3 (MIT and Apache-2.0).
