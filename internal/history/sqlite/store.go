package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlitedrv "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	drv = "sqlite"

	histCols = `(id, id_num, exec_ns, env, req_name, file_path, file_norm, method, url, status,
		status_code, dur_ns, snippet, req_text, descr, tags_json, prof_json, trace_json, cmp_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Regular writes replace by ID so reruns can refresh the same row,
	// while legacy migration keeps the first copy and skips duplicates.
	qReplace = `INSERT OR REPLACE INTO hist ` + histCols
	qIgnore  = `INSERT OR IGNORE INTO hist ` + histCols
)

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
	return s.ensure()
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
	if err := s.ensure(); err != nil {
		return err
	}

	r, err := mkRow(e)
	if err != nil {
		return err
	}

	if _, err = insertRow(s.db, qReplace, &r); err != nil {
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
	name = history.NormalizeWorkflowName(name)
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
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	n := history.NormPath(path)
	if n == "" {
		return nil, nil
	}
	return s.rows(`WHERE file_norm = ?`, []any{n})
}

func (s *Store) Delete(id string) (bool, error) {
	if err := s.ensure(); err != nil {
		return false, err
	}

	res, err := s.db.Exec(`DELETE FROM hist WHERE id = ?`, id)
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
	if err := s.ensure(); err != nil {
		return nil, err
	}

	q := `SELECT
		id, id_num, exec_ns, env, req_name, file_path, method, url, status, status_code, dur_ns,
		snippet, req_text, descr, tags_json, prof_json, trace_json, cmp_json
	FROM hist`
	if strings.TrimSpace(where) != "" {
		q += " " + where
	}
	// This ordering is shared across list and migration paths so
	// every caller sees the same history precedence for tied timestamps.
	q += ` ORDER BY exec_ns DESC, id_num DESC, id DESC`

	rs, err := s.db.Query(q, args...)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassHistory, err, "query history rows")
	}
	defer func() { _ = rs.Close() }()

	es := make([]history.Entry, 0, history.InitCap)
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

func scanRow(rs *sql.Rows) (history.Entry, error) {
	var (
		id, env, reqName, filePath, method, url, status, snippet, reqText, descr string
		idNum, execNs, statusCode, durNs                                         int64
		tagsJSON, profJSON, traceJSON, cmpJSON                                   []byte
	)
	err := rs.Scan(
		&id,
		&idNum,
		&execNs,
		&env,
		&reqName,
		&filePath,
		&method,
		&url,
		&status,
		&statusCode,
		&durNs,
		&snippet,
		&reqText,
		&descr,
		&tagsJSON,
		&profJSON,
		&traceJSON,
		&cmpJSON,
	)
	if err != nil {
		return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "scan history row")
	}

	e := history.Entry{
		ID:          id,
		ExecutedAt:  nsToTime(execNs),
		Environment: env,
		RequestName: reqName,
		FilePath:    filePath,
		Method:      method,
		URL:         url,
		Status:      status,
		StatusCode:  int(statusCode),
		Duration:    time.Duration(durNs),
		BodySnippet: snippet,
		RequestText: reqText,
		Description: descr,
	}

	if len(tagsJSON) > 0 {
		tags, err := dec[[]string](tagsJSON)
		if err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history tags")
		}
		e.Tags = tags
	}
	if len(profJSON) > 0 {
		p, err := dec[history.ProfileResults](profJSON)
		if err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history profile")
		}
		e.ProfileResults = &p
	}
	if len(traceJSON) > 0 {
		t, err := dec[history.TraceSummary](traceJSON)
		if err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history trace")
		}
		e.Trace = &t
	}
	if len(cmpJSON) > 0 {
		c, err := dec[history.CompareEntry](cmpJSON)
		if err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history compare")
		}
		e.Compare = &c
	}

	return e, nil
}

func mkRow(e history.Entry) (row, error) {
	r := row{
		id:         e.ID,
		idNum:      parseIDNum(e.ID),
		execNs:     timeToNS(e.ExecutedAt),
		env:        e.Environment,
		reqName:    e.RequestName,
		filePath:   e.FilePath,
		fileNorm:   history.NormPath(e.FilePath),
		method:     e.Method,
		url:        e.URL,
		status:     e.Status,
		statusCode: int64(e.StatusCode),
		durNs:      int64(e.Duration),
		snippet:    e.BodySnippet,
		reqText:    e.RequestText,
		descr:      e.Description,
	}

	var err error
	if len(e.Tags) > 0 {
		r.tagsJSON, err = enc(e.Tags)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history tags")
		}
	}
	if e.ProfileResults != nil {
		r.profJSON, err = enc(e.ProfileResults)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history profile")
		}
	}
	if e.Trace != nil {
		r.traceJSON, err = enc(e.Trace)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history trace")
		}
	}
	if e.Compare != nil {
		r.cmpJSON, err = enc(e.Compare)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history compare")
		}
	}

	return r, nil
}

type row struct {
	id         string
	idNum      int64
	execNs     int64
	env        string
	reqName    string
	filePath   string
	fileNorm   string
	method     string
	url        string
	status     string
	statusCode int64
	durNs      int64
	snippet    string
	reqText    string
	descr      string
	tagsJSON   []byte
	profJSON   []byte
	traceJSON  []byte
	cmpJSON    []byte
}

func (r *row) args() []any {
	return []any{
		r.id, r.idNum, r.execNs, r.env, r.reqName, r.filePath, r.fileNorm,
		r.method, r.url, r.status, r.statusCode, r.durNs, r.snippet,
		r.reqText, r.descr, r.tagsJSON, r.profJSON, r.traceJSON, r.cmpJSON,
	}
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertRow(db execer, q string, r *row) (sql.Result, error) {
	return db.Exec(q, r.args()...)
}

func parseIDNum(id string) int64 {
	// Non numeric IDs still work because query ordering falls back to
	// text ID after this value, so old rows remain deterministic.
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func timeToNS(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func nsToTime(ns int64) time.Time {
	if ns <= 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (s *Store) ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.p), 0o755); err != nil {
		return diag.WrapAs(diag.ClassFilesystem, err, "create history dir")
	}

	// Opening is lazy so commands that never touch history do not pay
	// the startup cost, but once opened this handle is reused safely.
	db, rec, err := s.openWithRecover()
	if err != nil {
		return err
	}

	s.db = db
	s.rec = rec
	return nil
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
	// Recovery only runs when the file exists and the failure strongly
	// looks like corruption, so regular open errors still surface.
	if !shouldRecover(s.p, err) {
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

func openReadyDB(dsn string) (*sql.DB, error) {
	// Opening does more than creating a handle.
	// It applies schema changes and runs an integrity check before returning.
	// A handle is returned only when the database is safe to use.
	db, err := sql.Open(drv, dsn)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassHistory, err, "open history db")
	}
	// SQLite behaves best here with a single connection because writes
	// are serialized and this avoids avoidable lock contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

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

func shouldRecover(path string, err error) bool {
	if !isCorruptErr(err) {
		return false
	}
	_, stErr := os.Stat(path)
	return stErr == nil
}

func isCorruptErr(err error) bool {
	if err == nil {
		return false
	}

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

func quarantineDB(path string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	dst := nextQuarantinePath(path + ".corrupt-" + ts)
	if err := moveIfExists(path, dst); err != nil {
		return "", err
	}
	// WAL and SHM files must move with the main file so SQLite never
	// tries to replay stale pages into the replacement database.
	if err := moveIfExists(path+"-wal", dst+"-wal"); err != nil {
		return "", err
	}
	if err := moveIfExists(path+"-shm", dst+"-shm"); err != nil {
		return "", err
	}
	return dst, nil
}

func nextQuarantinePath(base string) string {
	p := base
	// Recovery can run multiple times in the same second, so numbered
	// suffixes keep each quarantined copy instead of overwriting one.
	for i := 1; i < 1000; i++ {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
		p = base + "." + strconv.Itoa(i)
	}
	return base + ".x"
}

func moveIfExists(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return diag.WrapAs(diag.ClassFilesystem, err, "move corrupted history file")
	}
	return nil
}
