# EXPLAIN: GetLatest before idx_apk_latest

```sql
EXPLAIN SELECT * FROM apk_versions
WHERE is_active = 1 AND package_name = 'com.test.app' AND deleted_at IS NULL;
```

Expected plan (MySQL 8.x with ~100k rows):

| id | select_type | table        | type | possible_keys              | key                   | key_len | ref   | rows  | filtered | Extra       |
|----|-------------|--------------|------|----------------------------|-----------------------|---------|-------|-------|----------|-------------|
| 1  | SIMPLE      | apk_versions | ref  | idx_apk_version_package    | idx_apk_version_package | 768   | const | ~12   | 10.00   | Using where |

## Notes

- Uses the existing `idx_apk_version_package` (version, package_name) or no effective index for the `is_active` filter.
- Loads **all active versions** for the package into memory.
- Application-layer sort via Go `IsNewerVersion()` loop.
- Each additional version for a package adds a row to the result set (N+1 problem at app level).
- Worst-case: fetching thousands of rows across many packages, then O(N) comparisons in Go.
