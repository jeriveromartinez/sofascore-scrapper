# APK Semver Migration Recovery Design

## Problem

Production failed while applying migration `000005_apk_semver_order`:

```text
Error 1071 (42000): Specified key was too long; max key length is 3072 bytes
```

The failure occurred while creating `idx_apk_latest` over the complete
`package_name` column and four numeric fields. Production's `package_name`
definition is wider than the repository baseline's `VARCHAR(191)`, so its
`utf8mb4` index contribution can exceed InnoDB's key limit.

MariaDB committed the preceding DDL before the index failed. The production
database therefore has the three semver columns but is recorded by
`golang-migrate` as version 5 with `dirty=true`. Changing the migration SQL
alone cannot recover this database because `golang-migrate` refuses to run any
further migration while it is dirty.

## Goals

- Make migration 5 safe when `package_name` is wider than 191 characters.
- Recover only the known, non-destructive partial state of migration 5.
- Preserve every APK row and the full `package_name` value.
- Continue applying later migrations after successful repair.
- Reject unknown dirty states instead of guessing or forcing them clean.

## Non-Goals

- Generic automatic repair for arbitrary dirty migrations.
- Truncating or narrowing `package_name`.
- Automatically rolling back schema or application data.
- Replacing the existing migration framework.

## Migration Change

Migration 5 will create `idx_apk_latest` using a prefix for the character
column:

```sql
CREATE INDEX idx_apk_latest ON apk_versions(
  package_name(191),
  is_active,
  version_major,
  version_minor,
  version_patch,
  id
);
```

The prefix bounds the `utf8mb4` contribution to 764 bytes while retaining the
numeric ordering fields. The query still compares the complete
`package_name`, so prefix collisions can increase rows examined but cannot
change results.

## Dirty Migration Recovery

`database.Migrate` already serializes migration work with the MariaDB advisory
lock. After constructing the `golang-migrate` instance and before calling
`Up`, it will inspect `Version()`.

No action is taken for a clean database. A dirty version other than 5 returns
the existing migration error. For exactly `version=5, dirty=true`, a focused
repair function will perform these checks and operations under the same lock:

1. Confirm `apk_versions` contains all three columns: `version_major`,
   `version_minor`, and `version_patch`.
2. Confirm every `version` still matches the migration's strict
   `MAJOR.MINOR.PATCH` expression.
3. Recalculate all three numeric columns idempotently from `version`.
4. Inspect `idx_apk_latest`.
5. If absent, determine the declared character length of `package_name`, choose
   `min(length, 191)`, and create the same ordered index with that prefix.
6. If present, accept it only when its ordered columns match the expected
   index; reject an incompatible index with the same name.
7. Call `Force(5)` only after all validation and repair operations succeed.
8. Call `Up` normally so migrations 6 and later run.

If `package_name` is not an indexable character column, any semver column is
missing, data is non-semver, or the existing index is incompatible, startup
fails with a specific diagnostic and leaves the database dirty.

## Failure Safety

The repair does not drop columns, indexes, tables, or rows. Recalculating
semver fields and creating the missing index are idempotent. The dirty flag is
cleared last, preventing a partially repaired schema from being reported as
healthy.

The deployment's existing artifact rollback remains unchanged. Because schema
repair is additive and migration 5 is non-destructive, the previous binary can
continue running if a later deployment step fails.

## Testing

MariaDB integration tests will use isolated temporary databases and cover:

- A clean schema at version 4 with `package_name VARCHAR(1024)`, proving the
  corrected migration 5 applies without exceeding the key limit.
- The observed partial state: semver columns present, index absent, and
  `schema_migrations` at dirty version 5. `Migrate` must repair it, clear dirty,
  create the prefix index, and apply all later migrations.
- A dirty version other than 5, which must remain dirty and return an error.
- Dirty migration 5 with missing semver columns or an incompatible existing
  index, which must fail without forcing the version clean.

Existing empty-database, idempotency, repository ordering, and deployment tests
must continue to pass.

## Operations

No direct database access is required for the known production state. The first
startup of the corrected binary performs the constrained repair and records a
non-secret log message before continuing migrations. Operators verify success
through `/health/ready` and the deployment workflow.
