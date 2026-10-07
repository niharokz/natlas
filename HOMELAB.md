# Natlas on nimory (private, gitignored)

How Natlas v2 runs on the homelab, and how it fits with Taskmaster. Share this file together with ARCHITECTURE.md when asking for help. It contains no passwords; those live only in `.env` on the server.

## Deployment

| Item | Value |
|---|---|
| Project folder | `/home/datar/docker/natlas` (a git clone) |
| Data folder | `/home/datar/data/notes/data`, mounted at `/data` (`NATLAS_DATA_DIR` in `.env`) |
| Data files | `event.md`, `subscription.md`, `birthdays.md`, `health.md`, `inventory.csv` |
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

## Living with Taskmaster

Taskmaster runs nightly as user 1000 in TZ Asia/Kolkata and rewrites the files with `yaml.safe_dump`:

| Time | Task | Effect on Natlas data |
|---|---|---|
| 01:40 | `subscription_update` | Moves a passed `next_due_date` forward one cycle. On the due day it adds the pending event `sub-<slug>-<date>`, titled "Subscription due: X". Sorts subscriptions (active first, then by due date). |
| 01:58 | `event_archive` | Moves closed one-time events older than 30 days to `event.md.bck`. |
| 02:00 Mon | `birthday_sync` | Rewrites `birthdays.md` from Google Contacts (`type`, `version`, `last_sync`, `people`). |
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

## Upgrading to 2.1.0 and cleaning the public repo (2026-10-07)

What the review found in the pushed repo (`gitlab/github/codeberg niharokz/natlas`, commit "natlas v1 added"):

- `.gitignore`, `.dockerignore`, `.env.example`, `.gitattributes` and `natlas.example.yml` were missing (dotfiles did not survive the copy; `natlas.example.yml` had been renamed to `natlas.yml`).
- Because of that, `.env`, `HOMELAB.md` and `natlas.yml` were committed and pushed.
  - `.env` is a **symlink** to `/home/datar/docker/.env`, so only that path was published, not the secrets in it.
  - `HOMELAB.md` (this file: domains, server paths, Taskmaster schedule) was published.
  - `natlas.yml` was identical to the example, so nothing private.

### 1. Keep the shared `.env`, but only pass Natlas its own variables

`.env` stays a symlink to the shared `/home/datar/docker/.env`. Since 2.1.0, `docker-compose.yml`
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

### 3. After deploying 2.1.0

- The app is now at `https://data.nihars.com/app`; `/` is the public about page. Caddy needs no change.
- Everyone is signed out once (new cookie name). On the phone, remove the old home-screen app and install it again from `/app`.
- `docker logs natlas` now shows every change and failed login with the visitor IP (from `CF-Connecting-IP` for tunnel traffic).
