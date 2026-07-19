# Rollback Procedures

## Migration Policy

All database migrations are embedded in the binary via `go:embed` and run automatically on application startup using `golang-migrate/v4`.

**Key rules:**
1. Every migration MUST have a corresponding `.down.sql` file.
2. Migrations are numbered sequentially (e.g., `000001`, `000002`, ...).
3. The application only runs forward (`Up`) migrations on startup.
4. Rollback (`.down.sql`) migrations are never executed automatically — they must be triggered manually.
5. **Destructive migrations must have `.down.sql` files that are intentionally no-ops** to prevent accidental reversal that would not restore data.

### Migration History

| Migration | Description | Destructive? | Rollback Safe? |
|---|---|---|---|
| `000001_baseline` | Initial schema (users, tournaments, teams, events, domains, devices, playback_logs, apk_versions, device_tournaments, global_tournament_configs, content_stats, crash_reports) | No | Yes (tables dropped) |
| `000002_event_status_and_timestamps` | Add `status_type` to events, fix timestamp units | No (schema change) | Yes (column dropped) |
| `000003_reset_playback_and_stats` | **Delete all playback and stats data** | **YES** | **No — down is empty** |
| `000005_apk_semver_order` | Add semver columns to APK versions | No | Yes (columns dropped, index dropped) |
| `000006_download_counter_flushes` | Create `download_counter_flushes` idempotency table | No | Yes (table dropped) |
| `000007_apk_publish_state` | Create `apk_upload_publications` table | No | Yes (table dropped) |

---

## Destructive Migration Warning

### Migration `000003_reset_playback_and_stats`

**This migration permanently deletes all data from `content_stats` and `playback_logs` tables.**

```sql
DELETE FROM `content_stats`;
DELETE FROM `playback_logs`;
```

**WARNING:**
- The `.down.sql` for this migration is empty. **There is no SQL-level rollback.**
- Running the down migration after the up migration has executed is a no-op — it will not restore data.
- **The only way to recover is from a database backup taken BEFORE the migration ran.**

### Pre-Migration Checklist (for 000003)

1. **Backup database:**
   ```bash
   mysqldump -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME \
     > sofascore_pre_000003_$(date +%Y%m%d_%H%M%S).sql
   ```

2. **Export affected tables separately:**
   ```bash
   mysqldump -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME \
     content_stats playback_logs \
     > sofascore_playback_stats_$(date +%Y%m%d_%H%M%S).sql
   ```

3. **Verify backup integrity:**
   ```bash
   grep -c "INSERT INTO" sofascore_playback_stats_*.sql
   ```

4. **Schedule maintenance window** — the migration runs at application startup.

5. **Deploy with the migration.** The application will automatically run the migration on first startup.

---

## Database Backup & Restore

### Backup

```bash
# Full backup
mysqldump -h $DB_HOST -u $DB_USER -p$DB_PASSWORD \
  --single-transaction --routines --triggers --events \
  $DB_NAME | gzip > sofascore_backup_$(date +%Y%m%d_%H%M%S).sql.gz

# Backup from within Docker
docker compose -f deployments/docker/compose.multi.yml exec mariadb \
  mysqldump -u root -p$DB_PASSWORD sofascore | gzip > backup.sql.gz
```

### Restore

```bash
# Restore from gzip backup
gunzip < sofascore_backup_YYYYMMDD_HHMMSS.sql.gz | \
  mysql -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME

# Restore to a new database (safer)
mysql -h $DB_HOST -u $DB_USER -p$DB_PASSWORD -e "CREATE DATABASE sofascore_restore"
gunzip < sofascore_backup_YYYYMMDD_HHMMSS.sql.gz | \
  mysql -h $DB_HOST -u $DB_USER -p$DB_PASSWORD sofascore_restore
```

### Docker-specific Restore

```bash
# Copy backup to container
docker cp backup.sql.gz sofascore-scrapper-mariadb-1:/tmp/

# Restore inside container
docker compose -f deployments/docker/compose.multi.yml exec mariadb \
  sh -c "gunzip < /tmp/backup.sql.gz | mysql -u root -p\$MARIADB_ROOT_PASSWORD sofascore"
```

---

## Rollback Procedure (Non-Destructive Migrations)

For non-destructive migrations (001, 002, 005, 006, 007), rollback follows the standard `golang-migrate` approach:

### Step 1: Stop all application instances
```bash
docker compose -f deployments/docker/compose.multi.yml stop backend-1 backend-2 backend-3
```

### Step 2: Run the down migration manually
```bash
# Install golang-migrate CLI if not present
# go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate -path ./migrations \
  -database "mysql://$DB_USER:$DB_PASSWORD@tcp($DB_HOST:$DB_PORT)/$DB_NAME" \
  down <N>
```

Where `<N>` is the number of migrations to roll back.

### Step 3: Roll back to a specific version
```bash
migrate -path ./migrations \
  -database "mysql://$DB_USER:$DB_PASSWORD@tcp($DB_HOST:$DB_PORT)/$DB_NAME" \
  force <VERSION>
```

### Step 4: Redeploy the previous application version

### Step 5: Start instances
```bash
docker compose -f deployments/docker/compose.multi.yml up -d backend-1 backend-2 backend-3
```

### Step 6: Verify health
```bash
curl http://localhost:8080/health/ready
```

---

## Flag Gates

The application supports the following runtime configuration gates:

| Gate | Mechanism | Default | Purpose |
|---|---|---|---|
| **Rate limiting** | Enabled if Redis is connected | Enabled | Cannot be disabled via flag; fail-open on Redis outage |
| **Scrape scheduler** | Always runs on startup | Enabled | Paused only when Redis locks are held by another instance |
| **Upload cleanup** | Enabled if Redis is connected | Enabled | Skipped if cleanup job or Redis is nil |
| **Download counter flush** | Enabled if counter is configured | Enabled | Skipped if counter is nil |
| **Stats aggregation** | Enabled if agg repository exists | Enabled | Skipped if repository is nil |

There are no feature flags in the current codebase. All features are always-on. To disable scraping (e.g., for a read-only maintenance window), stop all instances except one — the distributed lock will ensure only one instance scrapes.

---

## Rollback Scenarios

### Scenario A: Bad migration (non-destructive)

**Problem:** Migration `000007` introduced a bug in `apk_upload_publications`.

**Rollback:**
1. Stop all instances.
2. Run `migrate down 1` to execute `000007_apk_publish_state.down.sql` (drops the table).
3. Deploy the previous application version.
4. Start instances.

### Scenario B: Destructive migration executed (000003)

**Problem:** Migration `000003` ran and deleted `content_stats` and `playback_logs` data.

**Rollback:**
1. The `.down.sql` is empty — there is no SQL-level undo.
2. **Restore from backup:**
   ```bash
   gunzip < sofascore_backup_pre_000003.sql.gz | \
     mysql -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME
   ```
3. Force the migration version back:
   ```bash
   migrate -path ./migrations \
     -database "mysql://$DB_USER:$DB_PASSWORD@tcp($DB_HOST:$DB_PORT)/$DB_NAME" \
     force 2
   ```
4. Deploy the previous application version.
5. Start instances.

### Scenario C: Application rollback (no DB changes)

**Problem:** New application version has a bug that doesn't involve migrations.

**Rollback:**
1. Stop all instances.
2. Revert to the previous Docker image / binary.
3. Start instances.
4. No migration rollback needed.

### Scenario D: Corrupted data (not a migration issue)

**Problem:** Accidental data corruption in a table.

**Rollback:**
1. **Do NOT run down migrations** — they drop tables/columns.
2. Restore only the affected table from a backup:
   ```bash
   # Extract single table from backup
   gunzip < backup.sql.gz | sed -n '/^-- Table structure.*affected_table/,/^-- Table structure/p' | \
     mysql -h $DB_HOST -u $DB_USER -p$DB_PASSWORD $DB_NAME
   ```

---

## Backup Schedule Recommendations

| Frequency | Type | Retention | Notes |
|---|---|---|---|
| Daily | Full (mysqldump) | 7 days | Cron: `0 2 * * *` |
| Weekly | Full (mysqldump) | 4 weeks | Cron: `0 3 * * 0` |
| Before any migration | Full + affected tables | Forever | Manual trigger |
| Before deployment | Full | 30 days | CI/CD integration |

---

## Emergency Contacts

- **Application owner:** See repository `CODEOWNERS` or GitHub collaborators.
- **Infrastructure:** See deployment platform documentation.
- **Upstream dependency status:**
  - Sofascore: https://www.sofascore.com
  - MariaDB: https://mariadb.org
  - Redis: https://redis.io
