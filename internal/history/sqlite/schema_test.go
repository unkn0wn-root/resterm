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
