DROP INDEX idx_apk_latest ON apk_versions;

ALTER TABLE apk_versions
  DROP COLUMN version_patch,
  DROP COLUMN version_minor,
  DROP COLUMN version_major;
