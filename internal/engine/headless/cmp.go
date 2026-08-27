package headless

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) executeCompare(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env vars.Environment,
) (*engine.CompareResult, error) {
	spec = core.NormalizeCompareSpec(spec)
	if spec == nil || len(spec.Environments) < 2 {
		return nil, fmt.Errorf("compare requires at least two environments")
	}
	targets, err := e.cfg.Catalog.CompareTargets(
		env.Selection(),
		spec.Group,
		spec.Baseline,
		spec.Environments,
	)
	if err != nil {
		return nil, err
	}
	pl, err := core.PrepareCompare(core.CompareInput{
		Doc:      doc,
		Request:  req,
		Targets:  targets,
		Group:    spec.Group,
		Baseline: spec.Baseline,
		Run:      core.RunMeta{Env: env},
	})
	if err != nil {
		return nil, err
	}
	cl := newCmpCollector()
	if err := core.RunCompare(ctx, e.rq, cl, pl); err != nil {
		return nil, err
	}
	out := e.buildCompareResult(req, spec, env, cl.rows)
	e.recordCompare(doc, req, out, env)
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
		Environment:    meta.Env,
		Profile:        meta.Profile,
		Selection:      out.Selection,
		Response:       cloneHTTP(out.Response),
		GRPC:           out.GRPC.Clone(),
		Stream:         cloneStream(out.Stream),
		Transcript:     copyBytes(out.Transcript),
		Err:            out.Err,
		Tests:          slices.Clone(out.Tests),
		ScriptErr:      out.ScriptErr,
		RuntimeSecrets: slices.Clone(out.RuntimeSecrets),
		Skipped:        out.Skipped,
		SkipReason:     strings.TrimSpace(out.SkipReason),
		Canceled:       canceled,
		Duration:       out.Timing.Elapsed(),
	}
	if canceled {
		row.Err = nil
	}
	return row
}

func (e *Engine) buildCompareResult(
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env vars.Environment,
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
		Group:       spec.Group,
		Environment: env.Label(),
		Selection:   env.Selection(),
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
		if spec != nil && strings.EqualFold(spec.Baseline, row.Name()) {
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

// rowEnvironment resolves the environment a compare row ran under. Rows carry
// their own selection, so history does not depend on rows and compare targets
// staying index aligned.
func (e *Engine) rowEnvironment(row engine.CompareRow, fallback vars.Environment) vars.Environment {
	if row.Selection.Empty() {
		return fallback
	}
	env, err := e.cfg.Catalog.Resolve(row.Selection)
	if err != nil {
		return fallback
	}
	return env
}

func (e *Engine) recordCompare(
	doc *restfile.Document,
	req *restfile.Request,
	out *engine.CompareResult,
	env vars.Environment,
) {
	hs := e.history()
	if hs == nil || req == nil || out == nil || len(out.Rows) == 0 {
		return
	}
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: out.Environment,
		EnvironmentSelection: history.EnvironmentSelection(
			out.Selection.Groups(),
		),
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      restfile.HistoryMethodCompare,
		URL:         req.URL,
		Status:      out.Summary,
		Duration:    compareDuration(out.Rows),
		RequestText: redactText(
			request.RenderRequestText(req),
			e.secretValues(doc, req, env, rowSecrets(out.Rows)...),
			!req.Metadata.AllowSensitiveHeaders,
		),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
		Compare: &history.CompareEntry{
			Baseline: out.Baseline,
			Group:    out.Group,
			Results:  make([]history.CompareResult, 0, len(out.Rows)),
		},
	}
	for _, row := range out.Rows {
		secs := e.secretValues(doc, req, e.rowEnvironment(row, env), row.RuntimeSecrets...)
		item := history.CompareResult{
			Environment: row.Environment,
			Profile:     row.Profile,
			EnvironmentSelection: history.EnvironmentSelection(
				row.Selection.Groups(),
			),
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
			item.Error = row.Err.Error()
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

func rowSecrets(rows []engine.CompareRow) []string {
	var out []string
	for _, row := range rows {
		out = append(out, row.RuntimeSecrets...)
	}
	return out
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
