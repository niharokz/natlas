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
