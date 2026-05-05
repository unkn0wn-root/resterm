package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestIsCorruptErrIntegrityCheckError(t *testing.T) {
	err := diag.WrapAs(
		diag.ClassHistory,
		&integrityCheckError{Check: "quick_check", Result: "database disk image is malformed"},
		"run history integrity check",
	)
	if !isCorruptErr(err) {
		t.Fatalf("expected integrity check error to be treated as corruption")
	}
}

func TestIsCorruptErrSQLiteCode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	if err := os.WriteFile(p, []byte("not-a-sqlite-db"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	_, err := openReadyDB(p)
	if err == nil {
		t.Fatalf("expected openReadyDB to fail")
	}
	if !isCorruptErr(err) {
		t.Fatalf("expected sqlite code path to be treated as corruption, got: %v", err)
	}
}

func TestIsCorruptErrRejectsNonCorruptionErrors(t *testing.T) {
	if isCorruptErr(nil) {
		t.Fatalf("expected nil error to be non-corrupt")
	}
	if isCorruptErr(diag.New(diag.ClassHistory, "unrelated failure")) {
		t.Fatalf("expected unrelated error to be non-corrupt")
	}
}
