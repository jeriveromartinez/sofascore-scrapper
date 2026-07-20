package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jeriveromartinez/sofascore-scrapper/migrations"
)

const (
	lockID                    = "590872375"
	apkSemverMigrationVersion = 5
	maxUnsignedBigInt         = "18446744073709551615"
)

var apkLatestIndexColumns = []string{
	"package_name",
	"is_active",
	"version_major",
	"version_minor",
	"version_patch",
	"id",
}

func repairDirtyAPKSemverMigration(ctx context.Context, conn *sql.Conn) error {
	var columnCount int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND COLUMN_NAME IN ('version_major', 'version_minor', 'version_patch')
	`).Scan(&columnCount); err != nil {
		return fmt.Errorf("inspect semver columns: %w", err)
	}
	if columnCount != 3 {
		return fmt.Errorf("expected 3 semver columns, found %d", columnCount)
	}
	var compatibleColumnCount int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND COLUMN_NAME IN ('version_major', 'version_minor', 'version_patch')
		  AND DATA_TYPE = 'bigint'
		  AND COLUMN_TYPE LIKE '% unsigned'
		  AND IS_NULLABLE = 'NO'
		  AND COLUMN_DEFAULT = '0'
	`).Scan(&compatibleColumnCount); err != nil {
		return fmt.Errorf("inspect semver column definitions: %w", err)
	}
	if compatibleColumnCount != 3 {
		return fmt.Errorf("semver columns have incompatible definitions: %d of 3 match BIGINT UNSIGNED NOT NULL DEFAULT 0", compatibleColumnCount)
	}

	var invalidCount int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM apk_versions
		WHERE version NOT REGEXP '^[0-9]+\\.[0-9]+\\.[0-9]+$'
	`).Scan(&invalidCount); err != nil {
		return fmt.Errorf("validate APK versions: %w", err)
	}
	if invalidCount != 0 {
		return fmt.Errorf("found %d non-semver APK versions", invalidCount)
	}
	var overflowCount int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM (
		  SELECT
		    SUBSTRING_INDEX(version, '.', 1) AS semver_major,
		    SUBSTRING_INDEX(SUBSTRING_INDEX(version, '.', 2), '.', -1) AS semver_minor,
		    SUBSTRING_INDEX(version, '.', -1) AS semver_patch
		  FROM apk_versions
		) AS semver_parts
		WHERE CHAR_LENGTH(semver_major) > 20
		   OR (CHAR_LENGTH(semver_major) = 20 AND BINARY semver_major > BINARY ?)
		   OR CHAR_LENGTH(semver_minor) > 20
		   OR (CHAR_LENGTH(semver_minor) = 20 AND BINARY semver_minor > BINARY ?)
		   OR CHAR_LENGTH(semver_patch) > 20
		   OR (CHAR_LENGTH(semver_patch) = 20 AND BINARY semver_patch > BINARY ?)
	`, maxUnsignedBigInt, maxUnsignedBigInt, maxUnsignedBigInt).Scan(&overflowCount); err != nil {
		return fmt.Errorf("validate APK semver bounds: %w", err)
	}
	if overflowCount != 0 {
		return fmt.Errorf("found %d APK versions where a semver component exceeds unsigned BIGINT maximum %s", overflowCount, maxUnsignedBigInt)
	}

	if _, err := conn.ExecContext(ctx, `
		UPDATE apk_versions SET
		  version_major = CAST(SUBSTRING_INDEX(version, '.', 1) AS UNSIGNED),
		  version_minor = CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(version, '.', 2), '.', -1) AS UNSIGNED),
		  version_patch = CAST(SUBSTRING_INDEX(version, '.', -1) AS UNSIGNED)
	`); err != nil {
		return fmt.Errorf("recalculate APK semver columns: %w", err)
	}

	var declaredLength sql.NullInt64
	if err := conn.QueryRowContext(ctx, `
		SELECT CHARACTER_MAXIMUM_LENGTH
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND COLUMN_NAME = 'package_name'
	`).Scan(&declaredLength); err != nil {
		return fmt.Errorf("inspect package_name length: %w", err)
	}
	if !declaredLength.Valid || declaredLength.Int64 < 1 {
		return fmt.Errorf("package_name is not an indexable character column")
	}
	prefixLength := declaredLength.Int64
	if prefixLength > 191 {
		prefixLength = 191
	}

	rows, err := conn.QueryContext(ctx, `
		SELECT COLUMN_NAME, SUB_PART, COLLATION, INDEX_TYPE, NON_UNIQUE, IGNORED
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'apk_versions'
		  AND INDEX_NAME = 'idx_apk_latest'
		ORDER BY SEQ_IN_INDEX
	`)
	if err != nil {
		return fmt.Errorf("inspect idx_apk_latest: %w", err)
	}
	var actualColumns []string
	var actualPrefix sql.NullInt64
	allAscending := true
	var actualIndexType string
	var actualNonUnique int
	var actualIgnored string
	for rows.Next() {
		var column string
		var subPart sql.NullInt64
		var collation sql.NullString
		var indexType string
		var nonUnique int
		var ignored string
		if err := rows.Scan(&column, &subPart, &collation, &indexType, &nonUnique, &ignored); err != nil {
			rows.Close()
			return fmt.Errorf("scan idx_apk_latest: %w", err)
		}
		if !collation.Valid || collation.String != "A" {
			allAscending = false
		}
		if len(actualColumns) == 0 {
			actualPrefix = subPart
			actualIndexType = indexType
			actualNonUnique = nonUnique
			actualIgnored = ignored
		}
		actualColumns = append(actualColumns, column)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close idx_apk_latest rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate idx_apk_latest: %w", err)
	}

	if len(actualColumns) != 0 {
		if len(actualColumns) != len(apkLatestIndexColumns) {
			return fmt.Errorf("idx_apk_latest has incompatible columns: %v", actualColumns)
		}
		for i := range apkLatestIndexColumns {
			if actualColumns[i] != apkLatestIndexColumns[i] {
				return fmt.Errorf("idx_apk_latest has incompatible columns: %v", actualColumns)
			}
		}
		if actualPrefix.Valid {
			if actualPrefix.Int64 != prefixLength {
				return fmt.Errorf("idx_apk_latest has incompatible package_name prefix: got %d, want %d", actualPrefix.Int64, prefixLength)
			}
		} else if declaredLength.Int64 > 191 {
			return fmt.Errorf("idx_apk_latest has incompatible package_name prefix: got full length %d, want %d", declaredLength.Int64, prefixLength)
		}
		if !allAscending {
			return fmt.Errorf("idx_apk_latest has incompatible direction: all columns must be ascending")
		}
		if actualIndexType != "BTREE" {
			return fmt.Errorf("idx_apk_latest has incompatible index type %q, want BTREE", actualIndexType)
		}
		if actualNonUnique != 1 {
			return fmt.Errorf("idx_apk_latest must be non-unique")
		}
		if actualIgnored != "NO" {
			return fmt.Errorf("idx_apk_latest is ignored")
		}
		return nil
	}

	statement := fmt.Sprintf(`
		CREATE INDEX idx_apk_latest ON apk_versions(
		  package_name(%d), is_active, version_major, version_minor, version_patch, id
		)
	`, prefixLength)
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("create idx_apk_latest: %w", err)
	}
	return nil
}

func releaseMigrationLock(conn *sql.Conn) error {
	var released sql.NullInt64
	if err := conn.QueryRowContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockID).Scan(&released); err != nil {
		return fmt.Errorf("release advisory lock: %w", err)
	}
	if !released.Valid || released.Int64 != 1 {
		return fmt.Errorf("release advisory lock returned %v, want 1", released)
	}
	return nil
}

func closeMigrationConnection(conn *sql.Conn) error {
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close migration connection: %w", err)
	}
	return nil
}

func Migrate(ctx context.Context, db *sql.DB) (retErr error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration connection: %w", err)
	}
	if err := conn.PingContext(ctx); err != nil {
		return errors.Join(fmt.Errorf("database ping: %w", err), closeMigrationConnection(conn))
	}
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockID).Scan(&acquired); err != nil {
		return errors.Join(fmt.Errorf("advisory lock: %w", err), closeMigrationConnection(conn))
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		if acquired.Valid && acquired.Int64 == 0 {
			return errors.Join(fmt.Errorf("advisory lock is already held"), closeMigrationConnection(conn))
		}
		return errors.Join(fmt.Errorf("advisory lock returned %v, want 1", acquired), closeMigrationConnection(conn))
	}

	src, err := iofs.New(migrations.FS(), ".")
	if err != nil {
		return errors.Join(fmt.Errorf("migration source: %w", err), releaseMigrationLock(conn), closeMigrationConnection(conn))
	}

	driver, err := mysql.WithConnection(ctx, conn, &mysql.Config{})
	if err != nil {
		return errors.Join(fmt.Errorf("migration driver: %w", err), releaseMigrationLock(conn), closeMigrationConnection(conn), src.Close())
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return errors.Join(fmt.Errorf("migration instance: %w", err), releaseMigrationLock(conn), driver.Close(), src.Close())
	}
	defer func() {
		retErr = errors.Join(retErr, releaseMigrationLock(conn))
		sourceErr, databaseErr := m.Close()
		if sourceErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close migration source: %w", sourceErr))
		}
		if databaseErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close migration database: %w", databaseErr))
		}
	}()

	version, dirty, versionErr := m.Version()
	if versionErr != nil && versionErr != migrate.ErrNilVersion {
		return fmt.Errorf("migration version: %w", versionErr)
	}
	if versionErr == nil && version == apkSemverMigrationVersion && dirty {
		if err := repairDirtyAPKSemverMigration(ctx, conn); err != nil {
			return fmt.Errorf("repair dirty migration %d: %w", version, err)
		}
		if err := m.Force(int(version)); err != nil {
			return fmt.Errorf("mark repaired migration %d clean: %w", version, err)
		}
		log.Printf("migrations: repaired dirty migration %d", version)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}
