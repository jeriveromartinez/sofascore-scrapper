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
# binary on startup, but migrations need a clean slate).
sudo mariadb -uroot -e "DROP DATABASE IF EXISTS iptv"
sudo mariadb -uroot -e "CREATE DATABASE iptv CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"

# Restore the dump.
gunzip -c /var/backups/iptv/iptv-<date>.sql.gz | sudo mariadb -uroot iptv

# Restart the service (it will run migrations if any are pending).
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

## Migration policy

- Every migration has a paired `.down.sql` file under `migrations/`.
- Migrations are embedded in the binary via `go:embed` and run
  automatically on startup using `golang-migrate/v4`.
- Migrations run **forward only** in production. `.down.sql` files
  exist for symmetry and for test fixtures; they are not invoked at
  runtime.
- Destructive migrations (e.g. `000003_reset_playback_and_stats`)
  have empty `.down.sql` files to prevent accidental reversal that
  would not restore data.

If `schema_migrations.dirty = true`, the next startup will refuse to
run. Clear the dirty flag after manual inspection:

```bash
sudo mariadb -uroot iptv -e "UPDATE schema_migrations SET dirty = 0"
sudo systemctl restart iptv.service
```
