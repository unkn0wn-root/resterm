package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sqlitedrv "modernc.org/sqlite"

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
	if util.SameFile(path, s.p) {
		return diag.WrapAs(
			diag.ClassHistory,
			errors.New("backup path must differ from history db path"),
			"backup history",
		)
	}
	notDB := diag.WrapAs(
		diag.ClassHistory,
		fmt.Errorf(
			"cannot replace %s because it is not a readable SQLite database. Remove it or pick another path",
			filepath.Base(path),
		),
		"backup history",
	)
	if st, err := os.Stat(path); err == nil && !st.Mode().IsRegular() {
		return notDB
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "create backup dir")
	}
	uri, err := fileURI(path)
	if err != nil {
		return err
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "backup history db")
	}
	defer func() { _ = conn.Close() }()
	created, err := createPrivate(path)
	if err != nil {
		return err
	}
	// SQLite writes the target itself and respects its locks and any leftover WAL.
	// A locked target is waited for, same as the live db.
	err = conn.Raw(func(dc any) error {
		b, err := dc.(interface {
			NewBackup(string) (*sqlitedrv.Backup, error)
		}).NewBackup(fmt.Sprintf("%s?_pragma=busy_timeout(%d)", uri, busyTimeout.Milliseconds()))
		if err != nil {
			return err
		}
		// Step(0) checks and locks the target but copies nothing.
		// The target turns private right there, before the first page of history is copied in.
		_, err = b.Step(0)
		if err == nil {
			makePrivate(path)
			_, err = b.Step(-1)
		}
		if err != nil {
			_ = b.Finish()
			return err
		}
		return b.Finish()
	})
	if err != nil {
		if created {
			_ = os.Remove(path)
		}
		if isCorruptErr(err) {
			return notDB
		}
		return diag.WrapAs(diag.ClassHistory, err, "backup history db")
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
