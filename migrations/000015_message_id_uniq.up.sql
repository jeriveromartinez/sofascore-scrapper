-- Add a message_id column (UUID v4) as the new UNIQUE lookup key for
-- delivery acknowledgement. Drops the old composite (push_message_id,
-- device_id) unique index; the ack path now looks up by message_id
-- alone (see internal/push/repository.go).
--
-- VARCHAR(36) is required: a UUID v4 is 128 bits (36 characters with
-- hyphens), which does NOT fit in a BIGINT UNSIGNED (64 bits).
--
-- The backfill step (step 2) is required because the column must be
-- NULLABLE first so that the unique index can be created without
-- violating NOT NULL on rows that already exist.

-- 1. Add message_id as NULLABLE so the unique constraint can be added
--    after rows are backfilled.
ALTER TABLE delivery_attempts ADD COLUMN message_id VARCHAR(36) NULL;

-- 2. Backfill existing rows with a unique UUID v4 per row.
UPDATE delivery_attempts SET message_id = UUID() WHERE message_id IS NULL;

-- 3. Enforce NOT NULL now that every row has a UUID.
ALTER TABLE delivery_attempts MODIFY COLUMN message_id VARCHAR(36) NOT NULL;

-- 4. Provide a dedicated supporting index for the FOREIGN KEY on
--    push_message_id (created in migration 13) BEFORE dropping
--    uq_push_device. Without this, MySQL refuses the DROP with
--    Error 1553 ("Cannot drop index ... needed in a foreign key
--    constraint") because uq_push_device is currently the only
--    index on push_message_id in this table.
CREATE INDEX idx_attempts_push_message ON delivery_attempts (push_message_id);

-- 5. Drop the old composite unique key (no longer the ack path).
DROP INDEX uq_push_device ON delivery_attempts;

-- 6. message_id is now the single UNIQUE key for ack lookups.
CREATE UNIQUE INDEX uq_message_id ON delivery_attempts (message_id);
