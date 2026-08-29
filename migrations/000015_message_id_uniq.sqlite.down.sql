-- SQLite variant of 000015_message_id_uniq down migration.
DROP INDEX IF EXISTS uq_message_id;
ALTER TABLE delivery_attempts DROP COLUMN message_id;
CREATE UNIQUE INDEX uq_push_device ON delivery_attempts (push_message_id, device_id);
