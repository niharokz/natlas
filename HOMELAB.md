# Natlas on nimory (private, gitignored)

How Natlas runs on the homelab, and how it fits with Taskmaster. Share this file together with ARCHITECTURE.md when asking for help. It contains no passwords; those live only in `.env` on the server.

## Deployment

| Item | Value |
|---|---|
| Project folder | `/home/datar/docker/natlas` (a git clone) |
| Data folder | `/home/datar/data/notes/data`, mounted at `/data` (`NATLAS_DATA_DIR` in `.env`) |
| Data files | `event.md`, `subscription.md`, `calendar.md` (read-only, from Kronos), `calendar-local.md` (Natlas's own calendar entries), `health.md`, `inventory.csv`, `homelab.md` (Architecture) |
| Container | `natlas`, user `1000:1000` (same as Taskmaster), read-only root filesystem, network `homelab` |
| Reached at | Caddy: `data.nihars.com` (Cloudflare Tunnel) and `data.nimory` (LAN, `tls internal`). Proxies to `natlas:8080`. No Caddy change was needed for v2. |
| Timezone | `TZ=Asia/Kolkata` in `.env` |

### First install (replacing v1)

This file is not in git; copy it into the clone by hand if you want it on the server.

```bash
cd /home/datar/docker
docker compose -f natlas/docker-compose.yml down     # stop v1
mv natlas natlas-v1                                  # keep until v2 is verified
git clone <repo> natlas && cd natlas
cp ../natlas-v1/.env .env                            # then add the new keys from .env.example
cp natlas.example.yml natlas.yml
ls -ln /home/datar/data/notes/data                   # v1 ran as root: check file owners
sudo chown 1000:1000 /home/datar/data/notes/data/*   # only if any are owned by root
docker compose run --rm natlas check                 # validate config
docker compose up -d --build
docker logs natlas                                   # "natlas listening on :8080 - plugins: [...]"
```

`.env` keys to add on top of v1's `NATLAS_USERNAME`, `NATLAS_PASSWORD` and `NATLAS_SECRET_KEY`: `NATLAS_DATA_DIR`, `TZ`, `PUID`, `PGID`, `NATLAS_NETWORK`.

Optional one-time cleanup: v1 wrote `priority: normal`, which Taskmaster flags as invalid.

```bash
sed -i 's/^\(\s*\)priority: normal$/\1priority: medium/' /home/datar/data/notes/data/event.md
```

### Updating later

```bash
cd /home/datar/docker/natlas && git pull && docker compose up -d --build
```

- **Editing a `plugin.yml` or `natlas.yml`:** run `docker compose restart natlas`. The `plugins/` folder is mounted.
- **Changing `.env`:** run `docker compose up -d` so the container is recreated.

### Publishing (GitHub, GitLab, Codeberg)

```bash
git remote add origin git@codeberg.org:<user>/natlas.git
git remote set-url --add --push origin git@codeberg.org:<user>/natlas.git
git remote set-url --add --push origin git@gitlab.com:<user>/natlas.git
git remote set-url --add --push origin git@github.com:<user>/natlas.git
ln -s ../../scripts/check-public.sh .git/hooks/pre-push    # blocks secrets / private words
```

Put these in `.private-words` (gitignored), one per line: your domain names, `datar`, `nimory`, real names, storage place names.

## Living with Kronos (formerly Taskmaster)

Kronos (the Go port of Taskmaster, same jobs and rules) runs nightly as user 1000 in TZ Asia/Kolkata:

| Time | Task | Effect on Natlas data |
|---|---|---|
| 01:40 | `subscription_update` | Moves a passed `next_due_date` forward one cycle. On the due day it adds the pending event `sub-<slug>-<date>`, titled "Subscription due: X". Sorts subscriptions (active first, then by due date). |
| 01:58 | `event_archive` | Moves closed one-time events older than 30 days to `event.md.bck`. |
| 02:20 | `calendar_sync` (Kronos) | Rewrites `calendar.md` in full from the Google Calendar API (7 days back … 365 ahead). Natlas shows it read-only and never writes it. `birthdays.md` is no longer updated (the old `birthday_sync`). |
| 02:10 | `event_update` | Overdue open one-time events become pending with today's date; recurring events are rolled forward or reopened (sets `last_done`); closed events older than a year are purged; events are sorted (active, then date, then priority). |
| 02:40 | `health_website` | Builds the health site from `health.md`. Field names and the "27 Sep 2026" date format are its contract. |
| 03:10 | `daily_update` | Writes `inbox/daily.md`, which feeds the 05:40 Telegram briefing. |

Natlas's side of the contract:
- **Done** only sets `status: closed`; Taskmaster handles recurrence.
- Natlas never sorts the files.
- **Overdue** means pending, or open with a date before today. This is the same rule as `daily_update.py`, so the Today screen and the Telegram agenda agree.
- New events always get all six fields Taskmaster checks: `id`, `title`, `date`, `status`, `frequency`, `priority`.
- Valid values: status `open` / `pending` / `closed`; priority `high` / `medium` / `low`; frequency `none` / `daily` / `weekly` / `monthly` / `quarterly` / `yearly`.
- Subscriptions are one flat `subscriptions:` list. Due-today items reach the Today screen through Taskmaster's pending event.
- If Taskmaster changes a record while it is open in Natlas (01:40–02:10), saving it gives "changed elsewhere, reload" instead of overwriting the change.

## Upgrading to 1.0.0 - bans, device key, config folder (2026-10-20)

Server:

```bash
cd /home/datar/docker/natlas
unzip -o patch/natlas-patch-1.0.0.zip
mkdir -p config && mv natlas.yml config/natlas.yml        # natlas.yml now lives in the config folder
sudo chown -R 1000:1000 config                            # PUID:PGID must be able to write bans.yml
openssl rand -hex 32                                      # → NATLAS_DEVICE_KEY=… in /home/datar/docker/.env
docker compose build && docker compose run --rm natlas check && docker compose up -d
docker logs natlas 2>&1 | grep "login:"                   # "5 failures ban a public IP for 7 days … remember device: on"
```

- Everyone is signed out once. Log in, then **This device** (side bar / More) → paste the device key → this browser stays signed in until logout. Do it once per browser (laptop, phone PWA).
- Optional in `.env`: `NATLAS_CONFIG_DIR` (default `./config`), `NATLAS_SESSION_HOURS=12`, `NATLAS_MAX_FAILURES=5`, `NATLAS_BAN_DAYS=7`.
- Banned yourself from mobile data? `docker exec natlas /natlas bans` and `docker exec natlas /natlas unban <ip>` (or `all`). Home Wi-Fi is never banned.

Android (keys already moved to `~/.local/share/keys/.natlas-keys`):

```bash
grep KEYSTORE_DIR android/.env                            # remove/empty NATLAS_KEYSTORE_DIR if it still points to ~/.natlas-keys
./android/build.sh quick                                  # → android/dist/natlas-1.0.0.apk (installs over 3.1.1)
```

Then Settings → **Config folder** → e.g. `notes/config/natlas` (synced, so settings survive a reinstall).

## Upgrading to 0.6.0 - Health from Samsung Health (2026-10-20)

```bash
cd /home/datar/docker/natlas
unzip -o patch/natlas-patch-3.1.0-health.zip
docker compose build && docker compose run --rm natlas check && docker compose up -d
./android/build.sh quick                                       # → android/dist/natlas-3.1.0.apk
```

Install the APK over 0.5.0 (same key, data kept), then on the phone:

1. Samsung Health → Settings → Health Connect → share everything.
2. Natlas → Settings → Health → **Allow access** → allow all, **past data** and **background**.
3. **Import all history** (once), keep **Sync every morning** at 07:00.
4. Add Steps / Sleep / Steps week / Weight widgets.

- New key `daily:` at the end of `/home/datar/data/notes/data/health.md`, one record per day (ISO dates, sorted). The rest of the file (meta, runs, gym) is untouched except `meta.weight`, `body_fat`, `height`, `last_updated`, which now follow Samsung Health.
- Goals: Health → Profile → Steps / Sleep / Exercise goal (stored as `goal_steps`, `goal_sleep_h`, `goal_active_min` in `meta:`).
- If health.nihars.com is built from `health.md` by a Kronos job, check that it ignores the new `daily:` key (it should only read `meta`, `runs`, `gym`).
- A phone sync and a web edit of health.md in the same minute → a conflict copy that nimory merges record by record.

## Upgrading to 0.5.0 - Android app (2026-10-19)

Server first (it now merges Syncthing conflict copies, `NATLAS_MAIN` defaults to on - nothing to add to `.env`):

```bash
cd /home/datar/docker/natlas
unzip -o patch/natlas-patch-3.0.0-android.zip
docker compose build && docker compose run --rm natlas check && docker compose up -d
docker logs natlas 2>&1 | tail -3
```

Then the APK, built on nimory with Docker (first build downloads the SDK, a few GB under `android/.cache`):

```bash
cd /home/datar/docker/natlas
cp android/.env.example android/.env && nano android/.env      # NATLAS_KEYSTORE_PASSWORD (save it in your password manager)
./android/build.sh keystore                                    # once - back up ~/.natlas-keys/natlas.jks
./android/build.sh quick                                       # 64-bit phone only, fastest; plain ./android/build.sh runs all tests
ls -lh android/dist/
```

Get `android/dist/natlas-3.0.0.apk` to the phone (Syncthing, scp, Telegram to yourself), install, then:

1. Allow **All files access**.
2. **Choose folder** → the synced `notes/data` folder (the one with `event.md`).
3. Settings → Security → Device lock (fingerprint). Settings → Sync → leave "This phone merges" **off** (nimory does it).
4. Battery: set Natlas to *Unrestricted* so widgets refresh at midnight.

- `android/.env`, `android/.cache/`, `android/dist/` and the jniLibs are gitignored; `~/.natlas-keys/` is outside the repo. Lose the key = uninstall/reinstall (no data lost; data lives in the notes folder).
- Kronos and the Telegram bot keep writing the same files; a phone edit and a Kronos write in the same minute end up as a conflict copy that nimory merges within 15 s.
- The phone app's package id is `app.natlas` (kept neutral for the public repo).

## Upgrading to 0.4.0 - Architecture (2026-10-19)

```bash
cd /home/datar/docker/natlas
unzip -o patch/natlas-patch-2.2.0-architecture.zip
cp patch/homelab.md /home/datar/data/notes/data/homelab.md      # first time only - never overwrite later edits
sed -i 's/^\(\s*\)- inventory$/\1- inventory\n\1- architecture/' natlas.yml   # or add "- architecture" by hand
docker compose build && docker compose run --rm natlas check && docker compose up -d
```

- `homelab.md` is now the homelab reference (replaces `/home/datar/docker/ARCHITECTURE.md` + `nimory.yaml`). Edit it in Natlas (Architecture) or Obsidian.
- The binary is ~3.6 MB bigger (Mermaid is built in); the diagram library is fetched by the browser only on the Architecture page.

## Upgrading to 0.3.1 - Calendar (2026-10-18)

Apply `natlas\patch\natlas-patch-0.3.1-calendar.zip` over the repo, then in the repo delete the old plugin and example:

```bash
git rm -r plugins/birthdays examples/data/birthdays.md
```

On the server (`natlas.yml` is not in git, and it still lists `birthdays`, which would stop Natlas from starting):

```bash
cd /home/datar/docker/natlas && git pull
sed -i 's/^\(\s*\)- birthdays$/\1- calendar/' natlas.yml
docker compose run --rm natlas check && docker compose up -d --build
docker logs natlas | tail -3   # "note calendar: …calendar-local.md does not exist yet" is expected once
```

- Your own entries go to `/home/datar/data/notes/data/calendar-local.md`, created on the first save. Kronos's `daily_update` does not read this file yet, so they are not in the Telegram briefing (a small Kronos patch would add it).
- `birthdays.md` in the data folder is unused now; delete it whenever you like.
- Edits to Google entries belong in Google Calendar (the sheet has an "Open in Google Calendar" button); they show up after the next 02:20 sync.

## Upgrading to 0.3.0 and cleaning the public repo (2026-10-07)

What the review found in the pushed repo (`gitlab/github/codeberg niharokz/natlas`, commit "natlas v1 added"):

- `.gitignore`, `.dockerignore`, `.env.example`, `.gitattributes` and `natlas.example.yml` were missing (dotfiles did not survive the copy; `natlas.example.yml` had been renamed to `natlas.yml`).
- Because of that, `.env`, `HOMELAB.md` and `natlas.yml` were committed and pushed.
  - `.env` is a **symlink** to `/home/datar/docker/.env`, so only that path was published, not the secrets in it.
  - `HOMELAB.md` (this file: domains, server paths, Taskmaster schedule) was published.
  - `natlas.yml` was identical to the example, so nothing private.

### 1. Keep the shared `.env`, but only pass Natlas its own variables

`.env` stays a symlink to the shared `/home/datar/docker/.env`. Since 0.3.0, `docker-compose.yml`
no longer uses `env_file:`; it lists the `NATLAS_*` variables (plus `TZ`) under `environment:`,
so the container only receives those, not the other services' secrets. Add a hash on the server:

```bash
cd /home/datar/docker/natlas
docker compose build
read -rs PW && printf '%s' "$PW" | docker run --rm -i natlas:local hash-password
# put NATLAS_PASSWORD_HASH=… in /home/datar/docker/.env and remove NATLAS_PASSWORD
docker compose run --rm natlas check && docker compose up -d
docker exec natlas /natlas version        # quick sanity check
```

Only the symlink's target path was published, and `.gitignore` now keeps `.env` out of git.

### 2. Rewrite the public history

There is a single commit, so the simplest clean-up is a fresh root commit, force-pushed to all three remotes. On the PC, in `D:\Downloads\nih.ar\natlas` (Git Bash):

```bash
git rm --cached -q .env HOMELAB.md natlas.yml     # stop tracking; files (and the .env symlink) stay on disk
git checkout --orphan clean
git add -A                                        # .gitignore now keeps the private files out
git status --short | grep -E '\.env$|HOMELAB|natlas\.yml$' && echo STOP || echo clean
git commit -m "natlas 2.1.0"
git branch -D main && git branch -m main
printf '%s\n' nihars datar nimory aziro > .private-words
ln -sf ../../scripts/check-public.sh .git/hooks/pre-push
sh scripts/check-public.sh
git push --force -u origin main                   # origin pushes to GitLab, GitHub and Codeberg
```

On GitLab, "main" may be a protected branch: allow force push for a minute (Settings → Repository → Protected branches) or unprotect and re-protect.
The old commit can stay reachable by its hash for a while on each host; if that matters, ask each host's support to purge it, or delete and re-create the repos.

### 3. After deploying 0.3.0

- The app is now at `https://data.nihars.com/app`; `/` is the public about page. Caddy needs no change.
- Everyone is signed out once (new cookie name). On the phone, remove the old home-screen app and install it again from `/app`.
- `docker logs natlas` now shows every change and failed login with the visitor IP (from `CF-Connecting-IP` for tunnel traffic).
