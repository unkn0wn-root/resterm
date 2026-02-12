package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/history"
)

func (s *Store) ExportJSON(path string) (int, error) {
	if err := s.ensure(); err != nil {
		return 0, err
	}

	var err error
	path, err = cleanPath(path, "export history")
	if err != nil {
		return 0, err
	}

	es, err := s.rows("", nil)
	if err != nil {
		return 0, err
	}

	data, err := enc(es)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "encode history export")
	}
	if err := writeFileAtom(path, data, 0o644); err != nil {
		return 0, err
	}
	return len(es), nil
}

func (s *Store) ImportJSON(path string) (int, error) {
	if err := s.ensure(); err != nil {
		return 0, err
	}

	var err error
	path, err = cleanPath(path, "import history")
	if err != nil {
		return 0, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "read history import")
	}
	es, err := dec[[]history.Entry](data)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "parse history import")
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "begin history import tx")
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
		if _, err = insertRow(tx, qReplace, &r); err != nil {
			return 0, errdef.Wrap(errdef.CodeHistory, err, "insert imported history row")
		}
		n++
	}

	if err := tx.Commit(); err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "commit history import tx")
	}
	return n, nil
}

func (s *Store) Backup(path string) error {
	// Backup writes a full SQLite snapshot to another file.
	// It rejects same-path targets to avoid self-overwrite.
	// The result is a standalone database that can be opened directly.
	if err := s.ensure(); err != nil {
		return err
	}

	var err error
	path, err = cleanPath(path, "backup history")
	if err != nil {
		return err
	}

	// The destination must be different from the live database path.
	// Removing an existing file is part of backup preparation.
	if samePath(path, s.p) {
		return errdef.Wrap(
			errdef.CodeHistory,
			errors.New("backup path must differ from history db path"),
			"backup history",
		)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "create backup dir")
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errdef.Wrap(errdef.CodeFilesystem, err, "remove existing backup")
	}

	// VACUUM INTO accepts a scalar expression for the output path.
	// Using a bound value avoids SQL text interpolation and escaping logic.
	if _, err := s.db.Exec(`VACUUM INTO ?`, path); err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "backup history db")
	}
	return nil
}

func cleanPath(path string, op string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errdef.Wrap(errdef.CodeHistory, errors.New("empty path"), "%s", op)
	}
	return filepath.Clean(path), nil
}

func writeFileAtom(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "create export dir")
	}

	// Writing in place can leave a truncated export on failure.
	// A temp file in the same directory keeps rename atomic.
	f, err := os.CreateTemp(dir, ".resterm-history-*.tmp")
	if err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "create export temp file")
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return errdef.Wrap(errdef.CodeFilesystem, err, "write export temp file")
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return errdef.Wrap(errdef.CodeFilesystem, err, "chmod export temp file")
	}
	if err := f.Close(); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "close export temp file")
	}
	if err := os.Rename(tmp, path); err != nil {
		return errdef.Wrap(errdef.CodeFilesystem, err, "replace export file")
	}
	return nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if p, err := filepath.Abs(a); err == nil {
		a = p
	}
	if p, err := filepath.Abs(b); err == nil {
		b = p
	}
	return a == b
}
