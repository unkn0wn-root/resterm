package sqlite

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func (s *Store) ExportJSON(path string) (int, error) {
	path, err := cleanPath(path, "export history")
	if err != nil {
		return 0, err
	}

	es, err := s.rows("", nil)
	if err != nil {
		return 0, err
	}

	data, err := json.Marshal(es)
	if err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "encode history export")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, diag.WrapAs(diag.ClassFilesystem, err, "create export dir")
	}
	if err := util.WriteFileAtomic(path, data, 0o600); err != nil {
		return 0, diag.WrapAs(diag.ClassFilesystem, err, "write export file")
	}
	return len(es), nil
}

func (s *Store) ImportJSON(path string) (int, error) {
	db, err := s.handle()
	if err != nil {
		return 0, err
	}

	path, err = cleanPath(path, "import history")
	if err != nil {
		return 0, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "read history import")
	}
	es, err := dec[[]history.Entry](data)
	if err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "parse history import")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "begin history import tx")
	}
	defer func() { _ = tx.Rollback() }()

	n := 0
	for _, e := range es {
		r, err := mkRow(e)
		if err != nil {
			return 0, err
		}
		// Import replaces by ID so a fresh export can correct stale rows
		// without asking users to clean the database first.
		if _, err = tx.Exec(qReplace, r.args()...); err != nil {
			return 0, diag.WrapAs(diag.ClassHistory, err, "insert imported history row")
		}
		n++
	}

	if err := tx.Commit(); err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "commit history import tx")
	}
	return n, nil
}

func (s *Store) Backup(path string) error {
	// Backup writes a full SQLite snapshot to another file.
	// It rejects same-path targets to avoid self-overwrite.
	// The result is a standalone database that can be opened directly.
	db, err := s.handle()
	if err != nil {
		return err
	}

	path, err = cleanPath(path, "backup history")
	if err != nil {
		return err
	}

	// The destination must be different from the live database path.
	if util.SamePath(path, s.p) {
		return diag.WrapAs(
			diag.ClassHistory,
			errors.New("backup path must differ from history db path"),
			"backup history",
		)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "create backup dir")
	}
	// VACUUM INTO won't overwrite a file. It writes into a temp dir and the result gets renamed.
	tmpDir, err := os.MkdirTemp(filepath.Dir(path), ".resterm-backup-*")
	if err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "create backup temp dir")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	tmp := filepath.Join(tmpDir, filepath.Base(path))

	// VACUUM INTO accepts a scalar expression for the output path.
	// Using a bound value avoids SQL text interpolation and escaping logic.
	if _, err := db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "backup history db")
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "make backup private")
	}
	if err := os.Rename(tmp, path); err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "replace backup")
	}
	return nil
}

func cleanPath(path string, op string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", diag.WrapAs(diag.ClassHistory, errors.New("empty path"), op)
	}
	return filepath.Clean(path), nil
}
