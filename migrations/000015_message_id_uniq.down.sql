-- Revert 000015_message_id_uniq: drop the UNIQUE index on message_id,
-- remove the column, drop the FK-supporting index added in the up,
-- and restore the original composite unique index (which is also the
-- supporting index for the FOREIGN KEY on push_message_id).
DROP INDEX uq_message_id ON delivery_attempts;
ALTER TABLE delivery_attempts DROP COLUMN message_id;
DROP INDEX idx_attempts_push_message ON delivery_attempts;
CREATE UNIQUE INDEX uq_push_device ON delivery_attempts (push_message_id, device_id);
