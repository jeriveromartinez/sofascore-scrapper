-- Revert users.password to longtext. Existing rows fit because we
-- never inserted anything wider than bcrypt's 60 bytes; this is
-- only here for rollback safety.
ALTER TABLE users MODIFY COLUMN password LONGTEXT NOT NULL;
