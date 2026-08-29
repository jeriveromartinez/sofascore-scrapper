-- Revert 000015_message_id_uniq: drop the UNIQUE index on message_id,
-- remove the column, and restore the original composite unique index.
DROP INDEX uq_message_id ON delivery_attempts;
ALTER TABLE delivery_attempts DROP COLUMN message_id;
CREATE UNIQUE INDEX uq_push_device ON delivery_attempts (push_message_id, device_id);
