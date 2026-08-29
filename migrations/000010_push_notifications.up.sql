-- Add per-user push-notifications feature toggle.
--
-- Default false so the user must explicitly opt in. The web dashboard
-- hides the push section and the REST endpoints return 403 until this
-- flag is flipped to true. notifications_enabled_at is a nullable
-- audit timestamp; NULL means the feature was never activated.
ALTER TABLE users ADD COLUMN notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN notifications_enabled_at TIMESTAMP NULL;
