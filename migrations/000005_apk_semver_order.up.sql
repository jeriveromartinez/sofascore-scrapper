DROP PROCEDURE IF EXISTS check_semver_preflight;

CREATE PROCEDURE check_semver_preflight()
BEGIN
  DECLARE invalid_count INT;
  DECLARE overflow_count INT;
  SELECT COUNT(*) INTO invalid_count FROM apk_versions
  WHERE `version` NOT REGEXP '^[0-9]+\\.[0-9]+\\.[0-9]+$';
  IF invalid_count > 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'preflight: apk_versions contains non-semver version strings. All versions must match MAJOR.MINOR.PATCH (non-negative integers only).';
  END IF;

  SELECT COUNT(*) INTO overflow_count
  FROM (
    SELECT
      SUBSTRING_INDEX(`version`, '.', 1) AS semver_major,
      SUBSTRING_INDEX(SUBSTRING_INDEX(`version`, '.', 2), '.', -1) AS semver_minor,
      SUBSTRING_INDEX(`version`, '.', -1) AS semver_patch
    FROM apk_versions
  ) AS semver_parts
  WHERE CHAR_LENGTH(semver_major) > 20
     OR (CHAR_LENGTH(semver_major) = 20 AND BINARY semver_major > BINARY '18446744073709551615')
     OR CHAR_LENGTH(semver_minor) > 20
     OR (CHAR_LENGTH(semver_minor) = 20 AND BINARY semver_minor > BINARY '18446744073709551615')
     OR CHAR_LENGTH(semver_patch) > 20
     OR (CHAR_LENGTH(semver_patch) = 20 AND BINARY semver_patch > BINARY '18446744073709551615');
  IF overflow_count > 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'preflight: apk_versions semver component exceeds unsigned BIGINT maximum 18446744073709551615.';
  END IF;
END;

CALL check_semver_preflight();
DROP PROCEDURE check_semver_preflight;

ALTER TABLE apk_versions
  ADD COLUMN version_major BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN version_minor BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN version_patch BIGINT UNSIGNED NOT NULL DEFAULT 0;

UPDATE apk_versions SET
  version_major = CAST(SUBSTRING_INDEX(`version`, '.', 1) AS UNSIGNED),
  version_minor = CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(`version`, '.', 2), '.', -1) AS UNSIGNED),
  version_patch = CAST(SUBSTRING_INDEX(`version`, '.', -1) AS UNSIGNED);

CREATE INDEX idx_apk_latest ON apk_versions(package_name(191), is_active, version_major, version_minor, version_patch, id);
