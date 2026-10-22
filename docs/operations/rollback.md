# Rollback Procedures

**Audience:** operators and sysadmins with access to the production
Ubuntu host.

Recovery procedure for the scores365-scrapper platform. The platform
has exactly **two durable pieces**:

| Piece | Path / name | Why it survives a redeploy |
|---|---|---|
| MariaDB (all relational data) | database `iptv` | Not touched by the package upgrade |
| Uploaded files and team logos | `/opt/iptv/apk_storage`, `/opt/iptv/image_storage` | `postinst` creates them if missing; it never deletes them |

The Go binary is stateless and is replaced by installing a lower
version. Nothing in it needs backing up.

> Binary path differs by install method. On a `.deb` host the binary is
> `/opt/iptv/iptv`. A native `go build` produces `./scores365-scrapper`.
> Commands below note which one applies.

## Database backup

Run on the production host:

```bash
sudo mariadb-dump \
    --single-transaction \
    --quick \
    --routines \
    --triggers \
    --events \
    iptv | gzip > /var/backups/iptv/iptv-$(date +%Y%m%d-%H%M%S).sql.gz
```

`--single-transaction` keeps InnoDB consistent without locking, so
this is safe to run against a live server. Suggested retention: 7
daily, 4 weekly, 6 monthly. Mirror the dumps to whatever off-host
storage the organisation already uses for database backups — a dump
that only lives on the production disk is not a backup.

Spot-check that a dump is not truncated:

```bash
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | head -50
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | tail -20
```

The dump should end with `-- Dump completed`. If it does not, the
`mariadb-dump` process was killed mid-run.

## Database restore

```bash
# Stop the writer first.
sudo systemctl stop iptv.service

sudo mariadb -uroot -e "DROP DATABASE IF EXISTS iptv"
sudo mariadb -uroot -e "CREATE DATABASE iptv CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"

gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | sudo mariadb -uroot iptv

sudo systemctl start iptv.service
curl -s http://127.0.0.1:8080/health/ready
```

The service re-runs `AutoMigrateAll` on start (see
[Schema policy](#schema-policy)), which creates any table or column
the dump did not contain. That makes an older dump forward-compatible:
restoring a backup taken before a release and then starting the newer
binary produces the current schema.

## Storage backup

```bash
sudo tar -czf /var/backups/iptv/storage-$(date +%Y%m%d-%H%M%S).tar.gz \
    /opt/iptv/apk_storage \
    /opt/iptv/image_storage
```

The service can stay up. Uploads are written to a temporary path and
renamed into place, so a file being read by `tar` is never a partial
file.

Losing `image_storage` costs team logos (re-fetchable from the
upstream API). Losing `apk_storage` costs unreleased APK builds — it
is not recoverable.

## Rollback to a previous `.deb`

`postinst` never overwrites an existing `/etc/iptv/env` and never
touches the storage directories, so a downgrade keeps the
configuration and the files. What it does **not** undo is schema: the
older binary expects the older schema, and AutoMigrate never drops
anything.

```bash
# Which versions apt knows about.
sudo apt-cache madison iptv

# Install the older package.
sudo apt install ./iptv_<older-version>_amd64.deb

sudo systemctl restart iptv.service
curl -s http://127.0.0.1:8080/health/ready
```

If `iptv.service` starts but immediately exits, read the log before
re-installing anything:

```bash
sudo journalctl -u iptv.service -n 100 --no-pager
```

The most common cause is a column the newer schema added and the older
code does not know about, or a renamed column. In that case restore
the database from a dump taken before the upgrade rather than trying
to make the old binary fit the new schema.

## Schema policy

The schema is owned by GORM `AutoMigrate`, implemented in
`internal/platform/database/automigrate.go` (`AutoMigrateAll`).
There is no SQL migration directory, no migration version table, and
no up/down scripts.

- Source of truth: the struct tags on the domain models under
  `internal/*/model.go`. The order of models passed to `AutoMigrateAll`
  is deliberate — foreign keys require parents before children.
- `AutoMigrateAll` runs on every boot of the server binary. `SKIP_MIGRATE=true`
  suppresses it (see [`runbook.md`](runbook.md) → First Boot).
- `AutoMigrateAll` also calls `PreMigrateLegacyEvents`, a one-off data
  backfill that runs before the loop and is a no-op on a clean
  database.
- **AutoMigrate is additive only.** It creates missing tables,
  columns, and indexes. It never drops or renames anything, and it
  does not narrow column types. Anything destructive — a `DROP`, a
  `RENAME`, a type change — must be applied to the live database by
  hand, outside the binary, before the release that needs it ships.
- Because AutoMigrate records nothing, there is no automated undo for
  a hand-applied destructive change. Keep the SQL used for it next to
  the release notes.

## Default admin after a destructive restore

`SeedDefaultAdmin` (`internal/seeder/admin.go`) runs from `app.New` on
every boot where `SKIP_MIGRATE` is unset. It is idempotent: it counts
the rows in `users` and returns without writing when the count is
greater than zero.

- **Restore from backup** — the dump carries the real user rows, so
  the seeder is a no-op and the operator accounts survive.
- **Deliberate wipe** (`DROP DATABASE` with no restore) — the seeder
  recreates the default admin on the next boot:
  - Email: `admin@local`
  - Password: the value of `DEFAULT_ADMIN_PASSWORD`; when that
    variable is unset the seeder generates a random password and never
    prints it
  - Role: `admin`

  Only the email is hardcoded in `internal/seeder/admin.go`. To
  register the first account through the invitation flow, mint a
  bootstrap invitation **before** the first normal boot. On a `.deb`
  host:

  ```bash
  sudo -u iptv bash -c 'set -a; . /etc/iptv/env; set +a; /opt/iptv/iptv bootstrap-invitation'
  ```

  With a native binary: `./scores365-scrapper bootstrap-invitation`.
  The command refuses to issue a token while `users` is non-empty, so
  the ordering matters.

The seeder also truncates `events`, `teams`, and `tournaments` when it
finds legacy SofaScore rows (rows with `source = 'sofascore'` or an
empty source). That guard is idempotent and only fires on data written
before the FotMob model reset.

See [`runbook.md`](runbook.md) → First Boot and → Bootstrap via
invitation for both paths.