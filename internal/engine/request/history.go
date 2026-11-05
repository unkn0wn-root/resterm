package request

import (
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) record(doc *restfile.Document, req *restfile.Request, res runResult) {
	hs := e.rt.History()
	if hs == nil || req == nil {
		return
	}
	// First, so a Flush on another goroutine never writes the entry without them.
	if l, ok := hs.(history.Learner); ok {
		l.Learn(res.RuntimeSecrets...)
	}
	runReq := res.Executed
	if runReq == nil {
		runReq = req
	}

	switch {
	case res.Skipped:
		e.recordSkipped(hs, doc, runReq, res)
	case res.Response != nil:
		e.recordHTTP(hs, doc, runReq, res)
	case res.GRPC != nil:
		e.recordGRPC(hs, doc, runReq, res)
	}
}

func (e *Engine) recordHTTP(
	hs history.Store,
	doc *restfile.Document,
	req *restfile.Request,
	res runResult,
) {
	resp := res.Response
	if hs == nil || req == nil || resp == nil {
		return
	}

	snip := history.BodySuppressed
	if !req.Metadata.NoLog {
		ct := ""
		if resp.Headers != nil {
			ct = resp.Headers.Get("Content-Type")
		}
		meta := binaryview.Analyze(resp.Body, ct)
		if meta.Kind == binaryview.KindBinary || !meta.Printable {
			snip = fmtBinary(meta, len(resp.Body))
		} else {
			snip = string(resp.Body)
		}
	}

	now := time.Now()
	ent := history.Entry{
		ID:          history.NewID(),
		ExecutedAt:  now,
		Environment: res.Env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(
			res.Env.Selection().Groups(),
		),
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.Status,
		StatusCode:  resp.StatusCode,
		Duration:    res.Elapsed,
		BodySnippet: snip,
		RequestText: HistoryText(req, res.RequestText),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
	}
	ent.Trace = history.NewTraceSummary(resp.Timeline, resp.TraceReport)
	_ = hs.Append(ent)
}

func (e *Engine) recordSkipped(
	hs history.Store,
	doc *restfile.Document,
	req *restfile.Request,
	res runResult,
) {
	if hs == nil || req == nil {
		return
	}
	txt := res.RequestText
	if strings.TrimSpace(txt) == "" {
		txt = renderRequestText(req)
	}
	snip := strings.TrimSpace(res.SkipReason)
	if snip == "" {
		snip = history.BodySkipped
	}
	now := time.Now()
	ent := history.Entry{
		ID:          history.NewID(),
		ExecutedAt:  now,
		Environment: res.Env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(
			res.Env.Selection().Groups(),
		),
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      "SKIPPED",
		BodySnippet: snip,
		RequestText: HistoryText(req, txt),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
	}
	_ = hs.Append(ent)
}

func (e *Engine) recordGRPC(
	hs history.Store,
	doc *restfile.Document,
	req *restfile.Request,
	res runResult,
) {
	resp := res.GRPC
	if hs == nil || req == nil || resp == nil {
		return
	}
	snip := resp.Message
	if req.Metadata.NoLog {
		snip = history.BodySuppressed
	}
	now := time.Now()
	ent := history.Entry{
		ID:          history.NewID(),
		ExecutedAt:  now,
		Environment: res.Env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(
			res.Env.Selection().Groups(),
		),
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.StatusCode.String(),
		StatusCode:  int(resp.StatusCode),
		Duration:    res.Elapsed,
		BodySnippet: snip,
		RequestText: HistoryText(req, res.RequestText),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
	}
	_ = hs.Append(ent)
}

func (e *Engine) secretValues(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	extra ...string,
) []string {
	return SecretSources{
		Doc:      doc,
		Req:      req,
		Env:      env,
		FilePath: e.filePath(doc),
		Files:    e.rt.Files(),
		Globals:  e.rt.Globals(),
		Extra:    extra,
	}.Secrets()
}

// Secrets returns the values a send of req under env masks.
func (e *Engine) Secrets(doc *restfile.Document, req *restfile.Request, env vars.Environment) []string {
	return e.secretValues(doc, req, ResolveEnvironment(env, doc, req))
}

// HistoryText masks sensitive header values in a request's text, unless the request logs them with
// @log-sensitive-headers.
func HistoryText(req *restfile.Request, text string) string {
	if req.Metadata.AllowSensitiveHeaders {
		return text
	}
	return header.RedactLines(text)
}

func fmtBinary(meta binaryview.Meta, n int) string {
	sz := fmtByteSize(int64(n))
	mime := strings.TrimSpace(meta.MIME)
	if mime != "" {
		return fmt.Sprintf("<binary body %s, %s>", sz, mime)
	}
	return fmt.Sprintf("<binary body %s>", sz)
}

func fmtByteSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

type runResult struct {
	Response       *httpx.Response
	GRPC           *grpcx.Response
	RuntimeSecrets []string
	RequestText    string
	Env            vars.ResolvedEnv
	Skipped        bool
	SkipReason     string
	Executed       *restfile.Request
	// Elapsed includes time spent polling and retrying.
	Elapsed time.Duration
}
