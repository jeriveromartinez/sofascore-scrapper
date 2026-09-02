-- Reverse migration 000017. Drops the `deleted_at` column and
-- its index from the three push tables added by the up migration.
--
-- Reversal is destructive only with respect to migration 000017;
-- it does not undo any data written through the column. Down
-- migrations are intended for dev/CI rollbacks, not production
-- downgrade flows.
ALTER TABLE `push_messages`
  DROP INDEX IF EXISTS `idx_push_messages_deleted_at`,
  DROP COLUMN IF EXISTS `deleted_at`;

ALTER TABLE `scheduled_pushes`
  DROP INDEX IF EXISTS `idx_scheduled_pushes_deleted_at`,
  DROP COLUMN IF EXISTS `deleted_at`;

ALTER TABLE `delivery_attempts`
  DROP INDEX IF EXISTS `idx_delivery_attempts_deleted_at`,
  DROP COLUMN IF EXISTS `deleted_at`;
