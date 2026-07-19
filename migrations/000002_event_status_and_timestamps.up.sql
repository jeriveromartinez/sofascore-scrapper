ALTER TABLE `events` ADD COLUMN `status_type` VARCHAR(32) NOT NULL DEFAULT '';

UPDATE `events`
SET `start_timestamp` = `start_timestamp` * 1000
WHERE `start_timestamp` > 0 AND `start_timestamp` < 1000000000000;

UPDATE `events`
SET `current_period_start_timestamp` = `current_period_start_timestamp` * 1000
WHERE `current_period_start_timestamp` > 0 AND `current_period_start_timestamp` < 1000000000000;

UPDATE `events`
SET `scraped_at` = `scraped_at` * 1000
WHERE `scraped_at` > 0 AND `scraped_at` < 1000000000000;

UPDATE `devices`
SET `last_seen` = `last_seen` * 1000
WHERE `last_seen` > 0 AND `last_seen` < 1000000000000;
