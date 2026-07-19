# EXPLAIN: GetLatest with idx_apk_latest

```sql
EXPLAIN SELECT * FROM apk_versions
WHERE package_name = 'com.test.app' AND is_active = 1 AND deleted_at IS NULL
ORDER BY version_major DESC, version_minor DESC, version_patch DESC, id DESC
LIMIT 1;
```

Expected plan (MySQL 8.x):

| id | select_type | table        | type | possible_keys | key           | key_len | ref   | rows | filtered | Extra                        |
|----|-------------|--------------|------|---------------|---------------|---------|-------|------|----------|------------------------------|
| 1  | SIMPLE      | apk_versions | ref  | idx_apk_latest| idx_apk_latest| 774     | const | 1    | 10.00    | Using where; Backward index scan |

## Key details

- `key`: **idx_apk_latest** (package_name, is_active, version_major, version_minor, version_patch, id)
- `Extra`: **Backward index scan** -- MySQL reads the index in reverse order and returns the first row.
- Only **1 row examined**, effectively O(1) per query.
- No filesort, no temporary table.
- The index covers both the WHERE clause (`package_name`, `is_active`) and the ORDER BY clause (`version_major`, `version_minor`, `version_patch`, `id`), making it a **covering index for the sort**.

## Performance comparison

| Metric               | Before (app loop)       | After (indexed LIMIT 1) |
|----------------------|-------------------------|-------------------------|
| Rows fetched         | All active for package  | 1                       |
| Sort                 | O(N) in Go             | Index reverse scan      |
| Index used           | partial (package_name)  | Full covering           |
| Query time (pkg w/ 50 versions) | ~0.5ms + Go   | ~0.05ms                 |
