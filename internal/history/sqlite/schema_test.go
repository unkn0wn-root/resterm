package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestParseIntegrityCheckResult(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		r := parseIntegrityCheckResult("ok")
		if r.status != integrityCheckStatusOK {
			t.Fatalf("expected OK status, got %v", r.status)
		}
		if r.detail != "" {
			t.Fatalf("expected empty detail for OK result, got %q", r.detail)
		}
	})

	t.Run("failed", func(t *testing.T) {
		r := parseIntegrityCheckResult("malformed page")
		if r.status != integrityCheckStatusFailed {
			t.Fatalf("expected failed status, got %v", r.status)
		}
		if r.detail != "malformed page" {
			t.Fatalf("expected failure detail to round trip, got %q", r.detail)
		}
	})

	t.Run("trimmed", func(t *testing.T) {
		r := parseIntegrityCheckResult("  ok  ")
		if r.status != integrityCheckStatusOK {
			t.Fatalf("expected OK status for trimmed result, got %v", r.status)
		}
	})
}

func TestMigrateSchemaFromV1(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")

	db, err := sql.Open(drv, p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := applyPragmas(db); err != nil {
		t.Fatalf("pragmas: %v", err)
	}
	if err := applyMigration(db, migs[0]); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	v, err := schemaVersion(db)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != 1 {
		t.Fatalf("expected schema v1, got %d", v)
	}

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	v, err = schemaVersion(db)
	if err != nil {
		t.Fatalf("schema version after migrate: %v", err)
	}
	if v != schemaVer {
		t.Fatalf("expected schema v%d, got %d", schemaVer, v)
	}

	ok, err := indexExists(db, "idx_hist_method")
	if err != nil {
		t.Fatalf("index check: %v", err)
	}
	if !ok {
		t.Fatalf("expected idx_hist_method to exist")
	}
}

func TestMigrateSchemaFromV2AddsEnvironmentSelection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.db")
	db, err := sql.Open(drv, p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := applyPragmas(db); err != nil {
		t.Fatalf("pragmas: %v", err)
	}
	for _, m := range migs[:2] {
		if err := applyMigration(db, m); err != nil {
			t.Fatalf("apply v%d: %v", m.ver, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO hist (id, exec_ns, status_code, dur_ns)
		VALUES ('old', 1, 0, 0)
	`); err != nil {
		t.Fatalf("insert v2 row: %v", err)
	}

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	ok, err := columnExists(db, "env_sel_json")
	if err != nil {
		t.Fatalf("column check: %v", err)
	}
	if !ok {
		t.Fatal("env_sel_json column was not added")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM hist WHERE id = 'old'`).Scan(&n); err != nil {
		t.Fatalf("query preserved row: %v", err)
	}
	if n != 1 {
		t.Fatalf("v2 row count = %d, want 1", n)
	}
}

func TestMigrateSchemaIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")

	db, err := sql.Open(drv, p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrate first: %v", err)
	}
	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrate second: %v", err)
	}
	v, err := schemaVersion(db)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if v != schemaVer {
		t.Fatalf("expected schema v%d, got %d", schemaVer, v)
	}
}

func indexExists(db *sql.DB, name string) (bool, error) {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`,
		name,
	).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func columnExists(db *sql.DB, name string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(hist)`)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			cid     int
			col     string
			kind    string
			notNull int
			def     any
			pk      int
		)
		if err := rows.Scan(&cid, &col, &kind, &notNull, &def, &pk); err != nil {
			return false, err
		}
		if col == name {
			return true, nil
		}
	}
	return false, rows.Err()
}
