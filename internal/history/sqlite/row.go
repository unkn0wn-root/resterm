package sqlite

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
)

const (
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
