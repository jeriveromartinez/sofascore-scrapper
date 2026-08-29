//go:build integration

package database

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestMigration15UpDownUp verifies the 000015_message_id_uniq migration
// (backfill NULLABLE column, enforce NOT NULL, swap unique index).
// Uses SQLite in-process so no external DB server is required.
func TestMigration15UpDownUp(t *testing.T) {
	// 1. Bootstrap a minimal delivery_attempts table that mimics the state
	//    after migration 13, with rows to exercise the backfill.
	schema := `
CREATE TABLE delivery_attempts (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at        TIMESTAMP NULL,
  push_message_id   BIGINT UNSIGNED NOT NULL,
  device_id         BIGINT UNSIGNED NOT NULL,
  state             VARCHAR(16) NOT NULL,
  failure_reason    VARCHAR(32) NULL,
  sent_at           TIMESTAMP NULL,
  acked_at          TIMESTAMP NULL,
  latency_ms        INT NULL
);
CREATE UNIQUE INDEX uq_push_device ON delivery_attempts (push_message_id, device_id);
`
	seedRows := `
INSERT INTO delivery_attempts (push_message_id, device_id, state) VALUES (1, 10, 'sent');
INSERT INTO delivery_attempts (push_message_id, device_id, state) VALUES (1, 11, 'sent');
INSERT INTO delivery_attempts (push_message_id, device_id, state) VALUES (2, 10, 'delivered');
`
	migration15Up := `
-- 1. Add message_id as NULLABLE.
ALTER TABLE delivery_attempts ADD COLUMN message_id VARCHAR(36) NULL;
-- 2. Backfill.
UPDATE delivery_attempts SET message_id = (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-4' ||
    substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', 1 + (abs(random()) % 4), 1) ||
    substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
) WHERE message_id IS NULL;
-- 3. Enforce NOT NULL (SQLite syntax).
ALTER TABLE delivery_attempts ALTER COLUMN message_id SET NOT NULL;
-- 4. Drop old composite unique key.
DROP INDEX uq_push_device;
-- 5. Create new UNIQUE index.
CREATE UNIQUE INDEX uq_message_id ON delivery_attempts (message_id);
`
	migration15Down := `
DROP INDEX IF EXISTS uq_message_id;
ALTER TABLE delivery_attempts DROP COLUMN message_id;
CREATE UNIQUE INDEX uq_push_device ON delivery_attempts (push_message_id, device_id);
`

	ctx := context.Background()

	db, err := sql.Open("sqlite3", "file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create schema and seed rows
	if _, err := db.ExecContext(ctx, schema); err != nil {
		t.Fatalf("create delivery_attempts table: %v", err)
	}
	if _, err := db.ExecContext(ctx, seedRows); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	// ── UP ──────────────────────────────────────────────────────────────────
	t.Log("Applying migration 15 up...")
	for _, stmt := range splitStatements(migration15Up) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("migration 15 up stmt failed:\n%s\nerror: %v", stmt, err)
		}
	}

	// Verify: message_id column is NOT NULL, has a UUID per row, UNIQUE index exists.
	var notNull int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('delivery_attempts') WHERE \"name\"='message_id' AND \"notnull\"=1").
		Scan(&notNull); err != nil {
		t.Fatalf("check notnull: %v", err)
	}
	if notNull != 1 {
		t.Fatal("message_id column is not NOT NULL after up migration")
	}

	var nullCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM delivery_attempts WHERE message_id IS NULL").
		Scan(&nullCount); err != nil {
		t.Fatalf("check null count: %v", err)
	}
	if nullCount != 0 {
		t.Fatalf("expected 0 NULL message_id rows after backfill, got %d", nullCount)
	}

	var uniqCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_index_list('delivery_attempts') WHERE name='uq_message_id'").
		Scan(&uniqCount); err != nil {
		t.Fatalf("check uq_message_id index: %v", err)
	}
	if uniqCount != 1 {
		t.Fatal("uq_message_id index not found after up migration")
	}

	var totalRows int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM delivery_attempts").
		Scan(&totalRows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if totalRows != 3 {
		t.Fatalf("expected 3 rows, got %d", totalRows)
	}

	var msgID string
	if err := db.QueryRowContext(ctx,
		"SELECT message_id FROM delivery_attempts LIMIT 1").
		Scan(&msgID); err != nil {
		t.Fatalf("read message_id: %v", err)
	}
	// UUID v4 format: 8-4-4-4-12 hex chars
	if len(msgID) != 36 {
		t.Fatalf("message_id %q is not a 36-char UUID", msgID)
	}

	t.Logf("UP migration OK — 3 rows with backfilled UUIDs (e.g. %s)", msgID)

	// ── DOWN ─────────────────────────────────────────────────────────────────
	t.Log("Applying migration 15 down...")
	for _, stmt := range splitStatements(migration15Down) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("migration 15 down stmt failed:\n%s\nerror: %v", stmt, err)
		}
	}

	// Verify: message_id column is gone, composite unique index is restored.
	var colCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('delivery_attempts') WHERE name='message_id'").
		Scan(&colCount); err != nil {
		t.Fatalf("check column gone: %v", err)
	}
	if colCount != 0 {
		t.Fatal("message_id column still exists after down migration")
	}

	var oldIdxCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_index_list('delivery_attempts') WHERE name='uq_push_device'").
		Scan(&oldIdxCount); err != nil {
		t.Fatalf("check uq_push_device index: %v", err)
	}
	if oldIdxCount != 1 {
		t.Fatal("uq_push_device index not restored after down migration")
	}

	t.Log("DOWN migration OK — message_id removed, uq_push_device restored")

	// ── UP again (idempotency) ───────────────────────────────────────────────
	t.Log("Applying migration 15 up again (idempotency)...")
	for _, stmt := range splitStatements(migration15Up) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("migration 15 re-up stmt failed:\n%s\nerror: %v", stmt, err)
		}
	}

	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('delivery_attempts') WHERE \"name\"='message_id' AND \"notnull\"=1").
		Scan(&notNull); err != nil {
		t.Fatalf("re-up notnull check: %v", err)
	}
	if notNull != 1 {
		t.Fatal("message_id not NOT NULL after re-up")
	}

	t.Log("RE-UP idempotency OK")
}

// splitStatements splits a multi-statement SQL string on semicolons,
// ignoring empty statements and trimming whitespace.
func splitStatements(sql string) []string {
	var stmts []string
	var cur string
	for _, ch := range sql {
		if ch == ';' {
			trimmed := trimCommentLines(cur)
			if trimmed != "" {
				stmts = append(stmts, trimmed)
			}
			cur = ""
		} else {
			cur += string(ch)
		}
	}
	trimmed := trimCommentLines(cur)
	if trimmed != "" {
		stmts = append(stmts, trimmed)
	}
	return stmts
}

// trimCommentLines removes comment-only lines before returning the statement.
func trimCommentLines(s string) string {
	var out string
	for _, line := range splitLines(s) {
		trimmed := trimSpace(line)
		if len(trimmed) > 0 && trimmed[:1] == "--" {
			continue
		}
		out += line + "\n"
	}
	return trimSpace(out)
}

func splitLines(s string) []string {
	var lines []string
	var cur string
	for _, ch := range s {
		if ch == '\n' {
			lines = append(lines, cur)
			cur = ""
		} else {
			cur += string(ch)
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func trimSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r' || s[j-1] == '\n') {
		j--
	}
	return s[i:j]
}
