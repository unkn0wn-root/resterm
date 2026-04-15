package headless

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func (e *Engine) executeCompare(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env string,
) (*engine.CompareResult, error) {
	pl, err := core.PrepareCompare(doc, req, spec, core.RunMeta{Env: e.env(env)})
	if err != nil {
		return nil, err
	}
	cl := newCmpCollector()
	if err := core.RunCompare(ctx, e.rq, cl, pl); err != nil {
		return nil, err
	}
	out := e.buildCompareResult(req, &pl.Spec, pl.Run.Env, cl.rows)
	e.recordCompare(doc, req, out)
	return out, nil
}

type cmpCollector struct {
	rows []engine.CompareRow
}

func newCmpCollector() *cmpCollector {
	return &cmpCollector{}
}

func (c *cmpCollector) OnEvt(_ context.Context, e core.Evt) error {
	if c == nil || e == nil {
		return nil
	}
	switch v := e.(type) {
	case core.CmpRowDone:
		c.rows = append(c.rows, compareRow(v.Row, v.Result))
	}
	return nil
}

func compareRow(meta core.RowMeta, out engine.RequestResult) engine.CompareRow {
	canceled := out.Err != nil && errors.Is(out.Err, context.Canceled)
	row := engine.CompareRow{
		Environment: strings.TrimSpace(meta.Env),
		Response:    cloneHTTP(out.Response),
		GRPC:        cloneGRPC(out.GRPC),
		Stream:      cloneStream(out.Stream),
		Transcript:  copyBytes(out.Transcript),
		Err:         out.Err,
		Tests:       append([]scripts.TestResult(nil), out.Tests...),
		ScriptErr:   out.ScriptErr,
		Skipped:     out.Skipped,
		SkipReason:  strings.TrimSpace(out.SkipReason),
		Canceled:    canceled,
	}
	if canceled {
		row.Err = nil
	}
	switch {
	case row.Response != nil:
		row.Duration = row.Response.Duration
	case row.GRPC != nil:
		row.Duration = row.GRPC.Duration
	}
	return row
}

func (e *Engine) buildCompareResult(
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env string,
	rows []engine.CompareRow,
) *engine.CompareResult {
	base := core.CompareBaseIndex(rows, compareBase(spec))
	if len(rows) > 0 {
		for i := range rows {
			rows[i].Summary = compareSummary(rows[base], rows[i])
			rows[i].Success = compareSuccess(rows[i])
		}
	}
	out := &engine.CompareResult{
		Baseline:    core.CompareBaseline(rows, compareBase(spec)),
		Environment: env,
		Rows:        rows,
	}
	allSkip := len(rows) > 0
	fail := false
	for _, row := range rows {
		if row.Canceled {
			out.Canceled = true
		}
		if !row.Skipped {
			allSkip = false
		}
		if row.Canceled || (!row.Skipped && !row.Success) {
			fail = true
		}
	}
	out.Skipped = allSkip
	out.Success = !out.Canceled && !out.Skipped && !fail
	out.Summary = compareRunSummary(req, rows, spec, out.Canceled)
	out.Report = compareReport(rows, spec)
	return out
}

func compareBase(spec *restfile.CompareSpec) string {
	if spec == nil {
		return ""
	}
	return spec.Baseline
}

func compareRunSummary(
	req *restfile.Request,
	rows []engine.CompareRow,
	spec *restfile.CompareSpec,
	canceled bool,
) string {
	lbl := "Compare"
	if title := engine.ReqTitle(req); title != "" {
		lbl = "Compare " + title
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		name := row.Environment
		if spec != nil && strings.EqualFold(spec.Baseline, name) {
			name += "*"
		}
		switch {
		case row.Canceled:
			name += "!"
		case row.Success:
			name += "✓"
		default:
			name += "✗"
		}
		parts = append(parts, name)
	}
	if len(parts) == 0 {
		return lbl
	}
	if canceled {
		return lbl + " canceled | " + strings.Join(parts, " ")
	}
	return lbl + " complete | " + strings.Join(parts, " ")
}

func compareReport(rows []engine.CompareRow, spec *restfile.CompareSpec) string {
	if len(rows) == 0 {
		return "Compare data unavailable"
	}
	base := core.CompareBaseline(rows, compareBase(spec))
	var b strings.Builder
	fmt.Fprintf(&b, "Baseline: %s\n\n", base)
	b.WriteString("Env\tStatus\tCode\tDuration\tDiff\n")
	for _, row := range rows {
		status, code := compareStatus(row)
		fmt.Fprintf(
			&b,
			"%s\t%s\t%s\t%s\t%s\n",
			row.Environment,
			status,
			code,
			row.Duration.Round(time.Millisecond),
			row.Summary,
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (e *Engine) recordCompare(
	doc *restfile.Document,
	req *restfile.Request,
	out *engine.CompareResult,
) {
	hs := e.history()
	if hs == nil || req == nil || out == nil || len(out.Rows) == 0 {
		return
	}
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      restfile.HistoryMethodCompare,
		URL:         req.URL,
		Status:      out.Summary,
		Duration:    compareDuration(out.Rows),
		RequestText: redactText(
			request.RenderRequestText(req),
			e.secretValues(doc, req, out.Environment),
			!req.Metadata.AllowSensitiveHeaders,
		),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
		Compare: &history.CompareEntry{
			Baseline: out.Baseline,
			Results:  make([]history.CompareResult, 0, len(out.Rows)),
		},
	}
	for _, row := range out.Rows {
		secs := e.secretValues(doc, req, row.Environment)
		item := history.CompareResult{
			Environment: row.Environment,
			Status:      compareHistoryStatus(row),
			Duration:    row.Duration,
			RequestText: ent.RequestText,
		}
		switch {
		case row.Canceled:
			item.Error = "canceled"
			item.BodySnippet = item.Error
		case row.Skipped:
			item.Error = row.SkipReason
			if item.Error == "" {
				item.Error = "skipped"
			}
			item.BodySnippet = item.Error
		case row.Err != nil:
			item.Error = errdef.Message(row.Err)
			item.BodySnippet = item.Error
		case row.Response != nil:
			item.StatusCode = row.Response.StatusCode
			item.BodySnippet = snippetHTTP(row.Response, req, secs)
		case row.GRPC != nil:
			item.StatusCode = int(row.GRPC.StatusCode)
			item.BodySnippet = snippetGRPC(row.GRPC, req, secs)
		default:
			item.BodySnippet = "No response captured"
		}
		if len(item.BodySnippet) > 2000 {
			item.BodySnippet = item.BodySnippet[:2000]
		}
		ent.Compare.Results = append(ent.Compare.Results, item)
	}
	_ = hs.Append(ent)
}

func compareDuration(rows []engine.CompareRow) time.Duration {
	var sum time.Duration
	for _, row := range rows {
		sum += row.Duration
	}
	return sum
}

func compareHistoryStatus(row engine.CompareRow) string {
	status, _ := compareStatus(row)
	return status
}
