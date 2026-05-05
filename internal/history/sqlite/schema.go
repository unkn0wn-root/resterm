package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

const (
	schemaVer = 2
)

type mig struct {
	ver int
	qs  []string
}

type integrityCheckStatus uint8

const (
	integrityCheckStatusOK integrityCheckStatus = iota + 1
	integrityCheckStatusFailed
)

type integrityCheckResult struct {
	status integrityCheckStatus
	detail string
}

func parseIntegrityCheckResult(v string) integrityCheckResult {
	s := strings.TrimSpace(v)
	if s == "ok" {
		return integrityCheckResult{status: integrityCheckStatusOK}
	}
	return integrityCheckResult{
		status: integrityCheckStatusFailed,
		detail: s,
	}
}

type integrityCheckError struct {
	Check  string
	Result string
}

func (e *integrityCheckError) Error() string {
	if e == nil {
		return ""
	}
	if e.Result == "" {
		return "history " + e.Check + " failed"
	}
	return "history " + e.Check + " failed: " + e.Result
}

// These are applied on every open because several settings are
// connection scoped and not persisted in the database file itself.
var pragmas = []string{
	`PRAGMA busy_timeout=5000;`,
	`PRAGMA journal_mode=WAL;`,
	`PRAGMA synchronous=FULL;`,
	`PRAGMA foreign_keys=ON;`,
	`PRAGMA temp_store=MEMORY;`,
}

var migs = []mig{
	{
		ver: 1,
		qs: []string{
			`CREATE TABLE IF NOT EXISTS meta (
				k TEXT PRIMARY KEY,
				v TEXT NOT NULL
			);`,
			`CREATE TABLE IF NOT EXISTS hist (
				id TEXT PRIMARY KEY,
				id_num INTEGER,
				exec_ns INTEGER NOT NULL,
				env TEXT,
				req_name TEXT,
				file_path TEXT,
				file_norm TEXT,
				method TEXT,
				url TEXT,
				status TEXT,
				status_code INTEGER NOT NULL,
				dur_ns INTEGER NOT NULL,
				snippet TEXT,
				req_text TEXT,
				descr TEXT,
				tags_json BLOB,
				prof_json BLOB,
				trace_json BLOB,
				cmp_json BLOB
			);`,
			`CREATE INDEX IF NOT EXISTS idx_hist_exec ON hist(exec_ns DESC, id_num DESC, id DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_hist_req ON hist(req_name, exec_ns DESC, id_num DESC, id DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_hist_url ON hist(url, exec_ns DESC, id_num DESC, id DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_hist_wf ON hist(method, req_name, exec_ns DESC, id_num DESC, id DESC);`,
			`CREATE INDEX IF NOT EXISTS idx_hist_file ON hist(file_norm, exec_ns DESC, id_num DESC, id DESC);`,
		},
	},
	{
		ver: 2,
		qs: []string{
			`CREATE INDEX IF NOT EXISTS idx_hist_method ON hist(method, exec_ns DESC, id_num DESC, id DESC);`,
		},
	},
}

func applyPragmas(db *sql.DB) error {
	for _, q := range pragmas {
		if _, err := db.Exec(q); err != nil {
			return diag.WrapAs(diag.ClassHistory, err, "apply history pragma")
		}
	}
	return nil
}

func schemaVersion(db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRow(`PRAGMA user_version;`).Scan(&v); err != nil {
		return 0, diag.WrapAs(diag.ClassHistory, err, "read history schema version")
	}
	return v, nil
}

func setSchemaVersion(tx *sql.Tx, v int) error {
	if v < 0 {
		return diag.Newf(diag.ClassHistory, "invalid history schema version: %d", v)
	}
	q := "PRAGMA user_version = " + strconv.Itoa(v)
	if _, err := tx.Exec(q); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "set history schema version")
	}
	return nil
}

func migrateSchema(db *sql.DB) error {
	// Schema upgrades are forward only and conservative.
	// If the file is newer than this build understands, it stops instead of guessing.
	// That protects user data from accidental downgrade writes.
	if err := applyPragmas(db); err != nil {
		return err
	}

	v, err := schemaVersion(db)
	if err != nil {
		return err
	}
	// Opening a database created by a newer build is rejected early.
	// Continuing would risk silent damage from unknown schema changes.
	if v > schemaVer {
		return diag.Newf(
			diag.ClassHistory,
			"history schema version %d is newer than supported %d",
			v,
			schemaVer,
		)
	}

	for _, m := range migs {
		if m.ver <= v {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return err
		}
		v = m.ver
	}

	if v != schemaVer {
		return diag.Newf(
			diag.ClassHistory,
			"history schema migration incomplete: got %d want %d",
			v,
			schemaVer,
		)
	}
	return nil
}

func applyMigration(db *sql.DB, m mig) error {
	// Each version step runs as a full transaction so either all DDL
	// for that step is visible, or none of it is.
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "begin history schema migration tx")
	}
	defer func() { _ = tx.Rollback() }()

	for _, q := range m.qs {
		if _, err := tx.Exec(q); err != nil {
			return diag.WrapAsf(diag.ClassHistory, err,
				"apply history schema migration v%d",
				m.ver,
			)
		}
	}
	if err := setSchemaVersion(tx, m.ver); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "commit history schema migration tx")
	}
	return nil
}

func checkDB(db *sql.DB, full bool) error {
	checkName := "quick_check"
	q := `PRAGMA quick_check;`
	// Quick checks are cheap enough for regular startup validation.
	// Full checks stay opt in because they are much slower on big files.
	if full {
		checkName = "integrity_check"
		q = `PRAGMA integrity_check;`
	}

	rs, err := db.Query(q)
	if err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "run history integrity check")
	}
	defer func() { _ = rs.Close() }()

	ok := false
	for rs.Next() {
		var v string
		if err := rs.Scan(&v); err != nil {
			return diag.WrapAs(diag.ClassHistory, err, "scan history integrity check")
		}
		r := parseIntegrityCheckResult(v)
		if r.status == integrityCheckStatusOK {
			ok = true
			continue
		}
		return diag.WrapAsf(diag.ClassHistory, &integrityCheckError{Check: checkName, Result: r.detail},
			"run history integrity check",
		)
	}
	if err := rs.Err(); err != nil {
		return diag.WrapAs(diag.ClassHistory, err, "iterate history integrity check")
	}
	if !ok {
		return diag.WrapAsf(diag.ClassHistory, &integrityCheckError{Check: checkName, Result: "empty result"},
			"run history integrity check",
		)
	}
	return nil
}
