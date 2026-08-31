-- Reverses 000016_crash_reports_embedded_fields.up.sql. The historical
-- `app_*` columns are left untouched; this drop only affects the
-- bare-name columns added in 16. Down-migrating a live system will
-- re-introduce the original `Error 1054: Unknown column 'name'`
-- failure on the next crash-report POST.

ALTER TABLE crash_reports
    DROP COLUMN IF EXISTS locale,
    DROP COLUMN IF EXISTS os_version,
    DROP COLUMN IF EXISTS platform,
    DROP COLUMN IF EXISTS environment,
    DROP COLUMN IF EXISTS build,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS name;
