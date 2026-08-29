-- SQLite variant of 000015_message_id_uniq.
-- UUID v4 generator: hex(randomblob(4)) for each segment, then
-- reassembled with the proper version-4 variant bits.
-- This expression is stable SQLite 3.7.4+ compatible.

-- 1. Add message_id as NULLABLE.
ALTER TABLE delivery_attempts ADD COLUMN message_id VARCHAR(36) NULL;

-- 2. Backfill existing rows with a unique UUID v4 per row.
--    Uses SQLite's randomblob() and hex() to build a RFC 4122 v4 UUID.
UPDATE delivery_attempts SET message_id = (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-4' ||
    substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', 1 + (abs(random()) % 4), 1) ||
    substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
) WHERE message_id IS NULL;

-- 3. Enforce NOT NULL now that every row has a UUID (SQLite syntax).
ALTER TABLE delivery_attempts ALTER COLUMN message_id SET NOT NULL;

-- 4. Drop the old composite unique key (no longer the ack path).
DROP INDEX uq_push_device;

-- 5. message_id is now the single UNIQUE key for ack lookups.
CREATE UNIQUE INDEX uq_message_id ON delivery_attempts (message_id);
