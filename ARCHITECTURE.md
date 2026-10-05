# Natlas architecture

> How Natlas v2 works inside. It is meant to be shared in a design or debugging conversation instead of the whole source: share this file, plus `HOMELAB.md` for deployment details (that file is gitignored), plus only the files being changed.
> It contains no secrets and no personal paths, so it is safe to commit.
> Keep the **Changelog** at the bottom current.

## 1. Design rules

1. **Flat files, no database.** YAML for structured data, CSV for tables. One file per plugin.
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
browser (Alpine, app.js) ──JSON──► net/http ServeMux
                                   ├── /api/login /logout /me ......... internal/auth
                                   ├── /api/app /dashboard /p/... ..... internal/api ──► internal/store ──► data file
                                   │                                        │               (yaml.Node / csv rows)
                                   │                                        └── internal/query (where, sort)
                                   └── / /assets/ /sw.js /manifest ..... web (go:embed)
```

The main pieces:

- **Process.** `cmd/natlas/main.go` loads the config, wires the routes, adds security headers and shuts down gracefully. It also has two subcommands: `natlas check` (validate the config) and `natlas health` (used by the Docker HEALTHCHECK).
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
| `internal/store` | The `Doc` interface: `Items`, `Create`, `Update`, `Delete`, `Move`, `Top`, `Save`. Implemented by `yamlDoc` and `csvDoc`. `File.Change(fn)` handles lock → read → fn → save. |
| `internal/query` | The where language (`eq ne in nin lt lte gt gte empty contains`, plus `any` / `all` / `not`). Values are compared by field type. `Sort` is stable and puts empty values last. |
| `internal/api` | Generic handlers, input cleaning and validation (`values.go`), Today screen (`dashboard.go`), quick-add parser (`quick.go`). |
| `internal/auth` | HMAC-SHA256 signed cookie `natlas_session` (`user|expiry`, valid 30 days), constant-time credential check, per-IP lockout (5 failures within 30 minutes). The lockout is kept in memory. |
| `internal/dates` | `Today()` in the container's TZ; parsing with a Go layout (ISO is always accepted too); tokens such as `today+7`; `CycleDays`, using the same rules as Taskmaster's `advance()`; `NextAnniversary`, where 29 Feb falls back to 28 Feb as in Taskmaster's `daily_update.py`. |
| `web` | Embeds the UI. `{{VERSION}}` (a hash of all web files) busts caches. Generates the PWA manifest, with one shortcut per plugin. `NATLAS_WEB_DIR` serves the files from disk during development. |

## 4. Storage details

### YAML (`store/yaml.go`)

- **Parsing.** The file is parsed into a `yaml.Node` tree. A top-level `updated:` key is removed while editing and written back as a fresh `updated: <now>\n\n` header on save, but only if the file had one.
- **Collections.** A collection is found by dotted `path` (for example `runs.planned`). `kind: list` expects a sequence and `kind: record` a mapping. Missing parts of the path are created on the first write.
- **Node to value.** `!!int` and `!!float` become `float64`, `!!bool` becomes `bool`, `!!null` becomes `nil`, and everything else, including timestamps and `65:30`, becomes a **string**. Sequences become `[]any`.
- **Update.** Only the given keys have their value nodes replaced, and line and head comments are carried over. New keys are appended. The record's position never changes.
- **Create.** The new mapping gets `id` first, then the fields in schema order. It goes at `new_at` (end or start), or at an explicit index (used by undo-delete).
- **Writing values.** `valueNode()` single-quotes any string that matches PyYAML's implicit resolvers (bool, int including sexagesimal, float, null, timestamp, merge). Whole floats are written as ints (`1200`, not `1200.0`).
- **Output.** The file is encoded with a 2-space indent. Lists come out as `  - item` (go-yaml style) rather than PyYAML's `- item`. Both are valid, and Taskmaster rewrites the file in its own style overnight anyway.

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

- **Structure.** One Alpine component, `natlas`, in `app.js`, and one `index.html`. Load order: `app.js` (defer), then `alpine.min.js` (defer). The component registers on `alpine:init`.
- **Routing.**
  - Screens use the hash: `#/today`, `#/p/{plugin}/{collection}`.
  - The edit sheet is a `<dialog>`. Opening it pushes a history entry, so the phone's Back button closes it.
- **Rendering.** Everything comes from the plugin definition: `list.title`, `subtitle`, `badges`, `search`, `filters`; `actions` with `primary`; `quick_add`; `summary`.
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
- **Theme.** `auto` follows the system; `dark` or `light` sets `data-theme` on `<html>`. Only the theme choice is kept in localStorage.
- **Service worker** (`sw.js`):
  - The app shell is cached per `{{VERSION}}`.
  - `GET /api/*` is network-first. When the network fails, the last good copy is served with an `X-Natlas-Offline` header, and the UI then shows "offline" and turns editing off.
  - Writes never go through the service worker, and `respondWith` always resolves to a real Response.
  - When a new version is waiting, an "Update" toast appears.
  - `?nosw` unregisters the worker and clears its caches.

## 7. Adding features

| Need | Where |
|---|---|
| A new kind of data | A new `plugins/<id>/plugin.yml`. No code. |
| A new field type | `config.fieldTypes`, `api.convert`, `store` (if it needs special storage), `app.js` `display()` and the sheet template. |
| A new condition operator | `query.validOps` and `fieldCond.Match`. |
| A new action kind | `config.Action` (plus validation in `prepare`) and `api.action` / `runSet` / `runMove`. |
| A new data format (for example JSON) | Implement `store.Doc`, then pick it by file extension in `File.Read` and `Plugin.Format`. |

## 8. Testing

Run `go test -mod=vendor ./...`. The tests cover:

- **store:** file untouched on read; comments and unknown fields kept; YAML 1.1 quoting; create, delete, undo and move; nested paths and record collections; CSV byte-identical rows and CRLF.
- **query:** operators, date tokens, select order, sorting.
- **dates:** anniversaries (including the leap day), cycles, formats.
- **config:** shipped plugins are valid; mistakes are collected and reported together; unknown keys are rejected.
- **api:** an end-to-end flow (login, Done, stale-revision 409, 422, quick add, read-only 403, cross-origin 403, health move, dashboard) and the quick-add parser.

UI changes are checked by hand with `scripts/dev.sh`, on a phone-sized viewport and on desktop.

## Changelog

- **2.0.0 (2026-10-05).** Rewrite in Go:
  - Plugins are now only YAML: no per-plugin Python, HTML or JS.
  - Editing moved to a list plus an edit sheet, with badge menus, one-tap actions, swipe, bulk edit, undo and quick add.
  - New Today screen.
  - Theme is now nss (light and dark).
  - PWA with offline read-only mode.
  - Record-level conflict detection.
  - Subscriptions use Taskmaster's flat `subscriptions:` list. v1 split it into `active:` / `cancelled:`, so the layout flipped back and forth every night.
  - Event priority values now match Taskmaster (`high` / `medium` / `low`).
