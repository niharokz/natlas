# Changelog

## 2.0.0 - 2026-10-05

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

### Upgrading from v1

- Secrets stay in `.env`. Create `natlas.yml` from `natlas.example.yml`; the v1 `tabs:` format no longer applies.
- `subscription.md` is now read and written as one `subscriptions:` list, which is the layout Taskmaster uses.
- Event priority is now `high` / `medium` / `low`. v1 wrote `normal`; existing values still display, and can be fixed once with
  `sed -i 's/^\(\s*\)priority: normal$/\1priority: medium/' event.md`.
