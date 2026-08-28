-- Link devices to a user-owned domain.
--
-- Nullable: a device registered before the push feature shipped stays
-- NULL and is excluded from push delivery. ON DELETE SET NULL keeps
-- existing devices working when an operator deletes a domain. The
-- index supports the push audience filter
-- (SELECT ... WHERE domain_id IN (...)).
ALTER TABLE devices ADD COLUMN domain_id BIGINT UNSIGNED NULL;
ALTER TABLE devices ADD CONSTRAINT fk_devices_domain
  FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE SET NULL;
CREATE INDEX idx_devices_domain_id ON devices(domain_id);
