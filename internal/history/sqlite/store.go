package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sqlitedrv "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const drv = "sqlite"

type Store struct {
	p string

	mu  sync.Mutex
	db  *sql.DB
	rec *RecoverInfo
}

type RecoverInfo struct {
	Path   string
	Backup string
	Cause  string
	At     time.Time
}

var _ history.Store = (*Store)(nil)
var _ history.MaintenanceStore = (*Store)(nil)

func New(path string) *Store {
	return &Store{p: path}
}

func (s *Store) Load() error {
	_, err := s.handle()
	return err
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	if err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "close history db")
	}
	return nil
}

func (s *Store) RecoveryInfo() *RecoverInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rec == nil {
		return nil
	}
	v := *s.rec
	return &v
}

func (s *Store) Append(e history.Entry) error {
	db, err := s.handle()
	if err != nil {
		return err
	}

	r, err := mkRow(e)
	if err != nil {
		return err
	}

	if _, err = db.Exec(qReplace, r.args()...); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "insert history row")
	}
	return nil
}

func (s *Store) Entries() ([]history.Entry, error) {
	return s.rows("", nil)
}

func (s *Store) ByRequest(id string) ([]history.Entry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	// Workflow runs use this same field for workflow names, so they are
	// excluded here to keep request history filtering precise.
	return s.rows(
		`WHERE method != ? AND (req_name = ? OR url = ?)`,
		[]any{restfile.HistoryMethodWorkflow, id, id},
	)
}

func (s *Store) ByWorkflow(name string) ([]history.Entry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	// Matching trims and lowercases both sides because saved names can
	// vary in spacing and case across edited files.
	return s.rows(
		`WHERE method = ? AND LOWER(TRIM(req_name)) = LOWER(TRIM(?))`,
		[]any{restfile.HistoryMethodWorkflow, name},
	)
}

func (s *Store) ByFile(path string) ([]history.Entry, error) {
	n := history.NormPath(path)
	if n == "" {
		return nil, nil
	}
	return s.rows(`WHERE file_norm = ?`, []any{n})
}

func (s *Store) Delete(id string) (bool, error) {
	db, err := s.handle()
	if err != nil {
		return false, err
	}

	res, err := db.Exec(`DELETE FROM hist WHERE id = ?`, id)
	if err != nil {
		return false, diag.WrapAs(diag.ClassHistory, err, "delete history row")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, diag.WrapAs(diag.ClassHistory, err, "history rows affected")
	}
	return n > 0, nil
}

func (s *Store) rows(where string, args []any) ([]history.Entry, error) {
	db, err := s.handle()
	if err != nil {
		return nil, err
	}

	q := `SELECT ` + cols + ` FROM hist`
	if where != "" {
		q += " " + where
	}
	// Every list uses this order. When times tie, the IDs decide.
	q += ` ORDER BY exec_ns DESC, id_num DESC, id DESC`

	rs, err := db.Query(q, args...)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassHistory, err, "query history rows")
	}
	defer func() { _ = rs.Close() }()

	es := []history.Entry{}
	for rs.Next() {
		e, err := scanRow(rs)
		if err != nil {
			return nil, err
		}
		es = append(es, e)
	}
	if err := rs.Err(); err != nil {
		return nil, diag.WrapAs(diag.ClassHistory, err, "iterate history rows")
	}
	return es, nil
}

// Callers use the returned handle because Close may reset s.db at any time.
func (s *Store) handle() (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}

	if err := os.MkdirAll(filepath.Dir(s.p), 0o700); err != nil {
		return nil, diag.WrapAs(diag.ClassFilesystem, err, "create history dir")
	}

	// Opening is lazy so commands that never touch history do not pay
	// the startup cost, but once opened this handle is reused safely.
	db, rec, err := s.openWithRecover()
	if err != nil {
		return nil, err
	}

	s.db = db
	s.rec = rec
	return db, nil
}

func (s *Store) openWithRecover() (*sql.DB, *RecoverInfo, error) {
	// This path first tries a normal open.
	// If the file looks corrupted, it moves the broken files aside and retries once.
	// That keeps history usable while preserving the original bytes for recovery.
	db, err := openReadyDB(s.p)
	if err == nil {
		return db, nil, nil
	}
	cause := err
	// Recovery only runs when the failure strongly looks like corruption, so regular open errors still surface.
	if !isCorruptErr(err) {
		return nil, nil, err
	}

	// The broken database is quarantined before reopening so callers can
	// keep running and still inspect or restore the old bytes later.
	bak, qErr := quarantineDB(s.p)
	if qErr != nil {
		return nil, nil, errors.Join(err, qErr)
	}

	db, err = openReadyDB(s.p)
	if err != nil {
		return nil, nil, errors.Join(
			diag.WrapAs(diag.ClassHistory, err, "open recovered history db"),
			fmt.Errorf("history db moved to %s", bak),
		)
	}
	rec := &RecoverInfo{
		Path:   s.p,
		Backup: bak,
		Cause:  cause.Error(),
		At:     time.Now().UTC(),
	}
	return db, rec, nil
}

func openReadyDB(path string) (*sql.DB, error) {
	// Opening does more than creating a handle.
	// It applies schema changes and runs an integrity check before returning.
	// A handle is returned only when the database is safe to use.
	if _, err := createPrivate(path); err != nil {
		return nil, err
	}
	makePrivate(path)
	uri, err := fileURI(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(drv, uri)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassHistory, err, "open history db")
	}
	// One connection serializes writes and keeps the pragmas, which are set per connection.
	db.SetMaxOpenConns(1)

	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := checkDB(db, false); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func isCorruptErr(err error) bool {
	var integErr *integrityCheckError
	if errors.As(err, &integErr) {
		return true
	}

	var se *sqlitedrv.Error
	if errors.As(err, &se) {
		code := se.Code()
		if code == sqlite3.SQLITE_IOERR_CORRUPTFS {
			return true
		}
		switch code & 0xff {
		case sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB:
			return true
		}
	}
	return false
}
