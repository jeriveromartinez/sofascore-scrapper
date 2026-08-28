-- Tighten users.password from longtext to VARCHAR(255).
--
-- Bcrypt output is always exactly 60 bytes. longtext is permissive
-- and invites regressions: a developer might temporarily store a
-- plain-text password thinking the column is permissive ("it's just
-- a test"). VARCHAR(255) is the smallest Bcrypt-compatible width with
-- headroom for future hashing algorithms (e.g. argon2id default is
-- ~95 chars, scrypt ~128 chars). NOT NULL is preserved.
ALTER TABLE users MODIFY COLUMN password VARCHAR(255) NOT NULL;
