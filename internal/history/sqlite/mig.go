package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/history"
)

const (
	metaMigJSON = "mig_history_json_v1"
)

func (s *Store) MigrateJSON(path string) (int, error) {
	// Legacy import is designed to run once and then get out of the way.
	// It marks completion even when there is nothing to import so startup stays predictable.
	// Existing SQLite rows always win over legacy JSON content.
	if err := s.ensure(); err != nil {
		return 0, err
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil
	}
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, errdef.Wrap(errdef.CodeHistory, err, "read legacy history")
	}

	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "begin history migration tx")
	}
	defer func() { _ = tx.Rollback() }()

	// Once this marker exists the legacy import is permanently done.
	// That keeps startup idempotent across restarts and upgrades.
	done, err := metaHas(tx, metaMigJSON)
	if err != nil {
		return 0, err
	}
	if done {
		return 0, nil
	}

	if len(bytes.TrimSpace(data)) == 0 {
		if err := metaSet(tx, metaMigJSON, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, errdef.Wrap(errdef.CodeHistory, err, "commit history migration tx")
		}
		return 0, nil
	}

	existing, err := rowCount(tx)
	if err != nil {
		return 0, err
	}
	// If SQLite already has rows we treat it as the source of truth and
	// only stamp completion, which avoids merging two diverged histories.
	if existing > 0 {
		if err := metaSet(tx, metaMigJSON, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, errdef.Wrap(errdef.CodeHistory, err, "commit history migration tx")
		}
		return 0, nil
	}

	es, err := dec[[]history.Entry](data)
	if err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "parse legacy history")
	}

	n := 0
	for _, e := range es {
		r, err := mkRow(e)
		if err != nil {
			return 0, err
		}
		// Duplicate IDs from legacy data are ignored so one bad file does
		// not abort the whole migration transaction.
		res, err := insertRow(tx, qIgnore, &r)
		if err != nil {
			return 0, errdef.Wrap(errdef.CodeHistory, err, "insert migrated history row")
		}
		ra, err := res.RowsAffected()
		if err == nil && ra > 0 {
			n++
		}
	}

	if err := trimMax(tx, s.max); err != nil {
		return 0, err
	}
	if err := metaSet(tx, metaMigJSON, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "commit history migration tx")
	}
	return n, nil
}

func rowCount(tx *sql.Tx) (int64, error) {
	var n int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM hist`).Scan(&n); err != nil {
		return 0, errdef.Wrap(errdef.CodeHistory, err, "count history rows")
	}
	return n, nil
}

func metaHas(tx *sql.Tx, key string) (bool, error) {
	var v string
	err := tx.QueryRow(`SELECT v FROM meta WHERE k = ?`, key).Scan(&v)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, errdef.Wrap(errdef.CodeHistory, err, "read history meta")
}

func metaSet(tx *sql.Tx, key, val string) error {
	_, err := tx.Exec(
		`INSERT INTO meta (k, v) VALUES (?, ?)
		 ON CONFLICT(k) DO UPDATE SET v = excluded.v`,
		key,
		val,
	)
	if err != nil {
		return errdef.Wrap(errdef.CodeHistory, err, "write history meta")
	}
	return nil
}
