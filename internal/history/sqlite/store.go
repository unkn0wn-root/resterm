package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
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

	// Reads and writes share this list. row.args and scanRow follow its order.
	cols = `id, id_num, exec_ns, env, env_sel_json, req_name, file_path, method, url, status,
		status_code, dur_ns, snippet, req_text, descr, tags_json, prof_json, trace_json, cmp_json`

	// file_norm only exists for ByFile lookups. It comes from file_path and is never read back.
	histCols = `(` + cols + `, file_norm)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

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
	var r row
	err := rs.Scan(
		&r.id, &r.idNum, &r.execNs, &r.env, &r.envSelJSON, &r.reqName, &r.filePath,
		&r.method, &r.url, &r.status, &r.statusCode, &r.durNs, &r.snippet,
		&r.reqText, &r.descr, &r.tagsJSON, &r.profJSON, &r.traceJSON, &r.cmpJSON,
	)
	if err != nil {
		return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "scan history row")
	}

	e := history.Entry{
		ID:          r.id,
		ExecutedAt:  nsToTime(r.execNs),
		Environment: r.env,
		RequestName: r.reqName,
		FilePath:    r.filePath,
		Method:      r.method,
		URL:         r.url,
		Status:      r.status,
		StatusCode:  int(r.statusCode),
		Duration:    time.Duration(r.durNs),
		BodySnippet: r.snippet,
		RequestText: r.reqText,
		Description: r.descr,
	}

	if len(r.envSelJSON) > 0 {
		if err := json.Unmarshal(r.envSelJSON, &e.EnvironmentSelection); err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode environment selection")
		}
	}
	if len(r.tagsJSON) > 0 {
		if err := json.Unmarshal(r.tagsJSON, &e.Tags); err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history tags")
		}
	}
	if len(r.profJSON) > 0 {
		if err := json.Unmarshal(r.profJSON, &e.ProfileResults); err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history profile")
		}
	}
	if len(r.traceJSON) > 0 {
		if err := json.Unmarshal(r.traceJSON, &e.Trace); err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history trace")
		}
	}
	if len(r.cmpJSON) > 0 {
		if err := json.Unmarshal(r.cmpJSON, &e.Compare); err != nil {
			return history.Entry{}, diag.WrapAs(diag.ClassHistory, err, "decode history compare")
		}
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
		method:     e.Method,
		url:        e.URL,
		status:     e.Status,
		statusCode: int64(e.StatusCode),
		durNs:      int64(e.Duration),
		snippet:    e.BodySnippet,
		reqText:    e.RequestText,
		descr:      e.Description,
		fileNorm:   history.NormPath(e.FilePath),
	}

	var err error
	if len(e.EnvironmentSelection) > 0 {
		r.envSelJSON, err = json.Marshal(e.EnvironmentSelection)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode environment selection")
		}
	}
	if len(e.Tags) > 0 {
		r.tagsJSON, err = json.Marshal(e.Tags)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history tags")
		}
	}
	if e.ProfileResults != nil {
		r.profJSON, err = json.Marshal(e.ProfileResults)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history profile")
		}
	}
	if e.Trace != nil {
		r.traceJSON, err = json.Marshal(e.Trace)
		if err != nil {
			return row{}, diag.WrapAs(diag.ClassHistory, err, "encode history trace")
		}
	}
	if e.Compare != nil {
		r.cmpJSON, err = json.Marshal(e.Compare)
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
	envSelJSON []byte
	reqName    string
	filePath   string
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
	fileNorm   string
}

func (r *row) args() []any {
	return []any{
		r.id, r.idNum, r.execNs, r.env, r.envSelJSON, r.reqName, r.filePath,
		r.method, r.url, r.status, r.statusCode, r.durNs, r.snippet,
		r.reqText, r.descr, r.tagsJSON, r.profJSON, r.traceJSON, r.cmpJSON, r.fileNorm,
	}
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

// The driver stops reading a plain path at "?". An escaped file URI keeps all of it.
func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", diag.WrapAs(diag.ClassFilesystem, err, "resolve db path")
	}
	p := filepath.ToSlash(abs)
	// A Windows drive path needs a leading slash, as in file:///C:/x.db.
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String(), nil
}

// History rows hold response bodies. SQLite gives new -wal and -shm files the mode of this file.
func createPrivate(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, diag.WrapAs(diag.ClassFilesystem, err, "create history file")
	}
	_ = f.Close()
	return true, nil
}

// Existing files are only changed by path and never opened.
// If this process closed a file SQLite has open, it would drop SQLite's locks on it.
func makePrivate(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		// Only the owner can change the mode. For anyone else, history opens as before.
		_ = os.Chmod(p, st.Mode().Perm()&^0o077)
	}
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
