-- Add the columns the GORM model `reporting.CrashReport` writes to
-- (the embedded `AppReport` and `DeviceReport` structs). Without an
-- explicit `embeddedPrefix` tag, GORM v1.31 produces column names
-- from the embedded field names alone: `name`, `version`, `build`,
-- `environment`, `platform`, `os_version`, `locale`. The original
-- baseline (000001) created these columns with the prefix
-- (`app_name`, `app_version`, ...), so every crash report from a
-- real device failed with `Error 1054: Unknown column 'name' in
-- 'INSERT INTO'`. See operations runbook: this column set is the
-- only one the deployed binary references.
--
-- We deliberately do NOT drop the historical `app_*` columns:
--   1. Some operational dashboards / ad-hoc queries still read them.
--   2. Dropping them now would require a coordinated model change
--      (adding `embeddedPrefix:app_` to the struct) so the next
--      binary writes to the new names. That refactor is intentionally
--      deferred to a separate change with a proper deprecation
--      window; until then, both column sets coexist and the bare
--      columns carry the live data.
--
-- `ADD COLUMN IF NOT EXISTS` requires MariaDB 10.0+ / MySQL 8.0.29+;
-- the project's supported runtime (Ubuntu 24.04 ships MariaDB 10.11)
-- satisfies both.

ALTER TABLE crash_reports
    ADD COLUMN IF NOT EXISTS name         LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS version      LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS build        LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS environment  LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS platform     LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS os_version   LONGTEXT NULL,
    ADD COLUMN IF NOT EXISTS locale       LONGTEXT NULL;
