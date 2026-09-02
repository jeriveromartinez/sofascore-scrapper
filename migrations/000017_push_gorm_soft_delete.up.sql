-- Add the `deleted_at` column to the three push tables whose
-- migrations (000012, 000013, 000014) pre-date the
-- `internal/push/model.go` rewrite to embed `gorm.Model`.
--
-- Without this column, every INSERT into these tables fails at
-- runtime with `Error 1054: Unknown column 'deleted_at' in
-- 'INSERT INTO'`, which is the same class of bug that PR #101
-- fixed for `crash_reports` and migration 000016 fixed for the
-- embedded AppReport / DeviceReport fields.
--
-- Idempotent (ADD COLUMN IF NOT EXISTS / ADD INDEX IF NOT EXISTS)
-- so re-running the chain test or re-applying on a database that
-- was partially migrated is safe. MariaDB 10.0+ / MySQL 8.0.29+
-- support both forms.
--
-- The down migration only reverses the columns and indexes added
-- here; it intentionally does NOT touch any historical schema
-- written by 000012 / 000013 / 000014.
ALTER TABLE `push_messages`
  ADD COLUMN IF NOT EXISTS `deleted_at` DATETIME(3) NULL,
  ADD INDEX IF NOT EXISTS `idx_push_messages_deleted_at` (`deleted_at`);

ALTER TABLE `scheduled_pushes`
  ADD COLUMN IF NOT EXISTS `deleted_at` DATETIME(3) NULL,
  ADD INDEX IF NOT EXISTS `idx_scheduled_pushes_deleted_at` (`deleted_at`);

ALTER TABLE `delivery_attempts`
  ADD COLUMN IF NOT EXISTS `deleted_at` DATETIME(3) NULL,
  ADD INDEX IF NOT EXISTS `idx_delivery_attempts_deleted_at` (`deleted_at`);
