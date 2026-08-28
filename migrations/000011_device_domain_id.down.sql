-- Revert device ↔ domain link.
ALTER TABLE devices DROP INDEX idx_devices_domain_id;
ALTER TABLE devices DROP FOREIGN KEY fk_devices_domain;
ALTER TABLE devices DROP COLUMN domain_id;
