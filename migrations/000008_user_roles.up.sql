ALTER TABLE users ADD COLUMN role VARCHAR(20) NOT NULL DEFAULT 'user';

-- Preserve existing behaviour: every account that existed before roles were
-- introduced was an implicit operator, so promote them all to admin. New
-- accounts created after this migration default to the least-privileged 'user'.
UPDATE users SET role = 'admin';
