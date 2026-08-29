-- Revert per-user push-notifications feature toggle.
ALTER TABLE users DROP COLUMN notifications_enabled_at;
ALTER TABLE users DROP COLUMN notifications_enabled;
