# Security

## Reporting a problem

Please report security issues privately, not in a public issue:
open a **confidential issue** at <https://gitlab.com/niharokz/natlas/-/issues/new>
(tick "This issue is confidential"). Include what you found, how to reproduce it,
and the version (`natlas version`, also shown at the bottom of the about page).

You should get a reply within a week. Fixes are released as a new version and
noted in `CHANGELOG.md`.

## Supported versions

Only the latest release is supported. Update with
`git pull && docker compose up -d --build`.

## What Natlas protects, and how

See "Security notes" in `README.md`. In short: single user, PBKDF2 password
hash, signed `__Host-` session cookie bound to the password, per-IP login
lockout, same-origin JSON writes, strict CSP and security headers, and a
non-root, read-only, capability-free container that can only write to your
data folder.

## Your part

- Use a long passphrase and store it as `NATLAS_PASSWORD_HASH`.
- Keep `.env` and `natlas.yml` out of git (`.gitignore` does this). A shared
  `.env` is fine: `docker-compose.yml` passes only the `NATLAS_*` variables and
  `TZ` into the container, never the rest of the file.
- Serve Natlas only over HTTPS, behind a proxy you control.
