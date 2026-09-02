# Rollback Procedures

Recovery for the platform. The platform has two durable pieces:
**MariaDB** (the only store with relational data) and the on-disk
storage at `/opt/iptv/apk_storage` and `/opt/iptv/image_storage`.
Back up both. The Go binary itself is stateless and gets replaced by
a downgrade.

## Database backup

Run on the production host, daily via cron:

```bash
sudo mariadb-dump \
    --single-transaction \
    --quick \
    --routines \
    --triggers \
    --events \
    iptv | gzip > /var/backups/iptv/iptv-$(date +%Y%m%d-%H%M%S).sql.gz
```

Suggested retention: 7 daily, 4 weekly, 6 monthly. Mirror to off-host
storage (S3, Backblaze, NFS, whatever the org already uses for DB
backups).

Verify the dump periodically:

```bash
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | head -50
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | wc -l
```

## Database restore

```bash
# Stop the service so it does not write during the restore.
sudo systemctl stop iptv.service

# Drop and recreate the database (the schema is recreated by the
# binary on startup via GORM AutoMigrate; no separate migrations to
# run).
sudo mariadb -uroot -e "DROP DATABASE IF EXISTS iptv"
sudo mariadb -uroot -e "CREATE DATABASE iptv CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"

# Restore the dump.
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | sudo mariadb -uroot iptv

# Restart the service (it will run AutoMigrate and seed the default admin).
sudo systemctl start iptv.service

# Verify.
curl -s http://127.0.0.1:8080/health/ready
```

## Storage backup

```bash
sudo tar -czf /var/backups/iptv/storage-$(date +%Y%m%d-%H%M%S).tar.gz \
    /opt/iptv/apk_storage \
    /opt/iptv/image_storage
```

The backend does not need to be stopped for this; uploads are atomic
at the file level.

## Rollback to a previous `.deb`

If a deployment introduced a regression, downgrade the package. The
postinst preserves `/etc/iptv/env` and stored data across upgrades, so
a downgrade is safe as long as the schema did not change between the
two versions (migrations only run forward; a downgrade does not undo
them).

```bash
# List installed versions known to apt.
sudo apt-cache madison iptv

# Pin to a specific older version.
sudo apt install iptv=<older-version>

# Restart to be sure.
sudo systemctl restart iptv.service
```

If the older version expects a different schema than the current DB
holds, restore the database from backup instead.

## Schema policy

Schema is managed by GORM AutoMigrate (see
`internal/platform/database/automigrate.go`). The canonical model
definitions live with the Go structs; there is no separate SQL
migration directory or version-tracking table.

- The schema source of truth is the struct tags (`gorm:"..."`) on the
  domain models under `internal/*/model.go`.
- AutoMigrate runs on every server boot unless `SKIP_MIGRATE=true`
  is set (see [`runbook.md`](runbook.md) → First Boot).
- AutoMigrate is **additive only**: it adds missing tables and
  columns but never drops or alters existing ones. Schema changes
  that need destructive operations (rename, type change, drop) must
  be applied manually to the live DB before deploying the new
  binary, then left in place. There is no rollback path because
  AutoMigrate does not record what it did.
- On a downgrade, the older binary expects the older schema. If the
  downgrade crosses a destructive-change boundary, restore the
  database from backup instead of relying on the binary's
  AutoMigrate.

## Default admin after a destructive restore

`SeedDefaultAdmin` runs on every normal boot (gated by
`SKIP_MIGRATE`). It is idempotent: it only creates the default
admin when the `users` table is empty. So:

- After a `DROP DATABASE` + restore-from-backup cycle, the restored
  dump carries the user data; the seeder is a no-op.
- After a `DROP DATABASE` + no-restore cycle (deliberate wipe),
  the seeder creates `admin@local` on the next boot. To avoid the
  hardcoded default password instead, run
  `./sofascore-scrapper bootstrap-invitation` *before* the first
  normal boot so the first human registers through the invitation
  flow.

See [`runbook.md`](runbook.md) → First Boot and Bootstrap via
invitation for both paths.
