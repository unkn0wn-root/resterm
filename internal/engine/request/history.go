package request

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var sensHdr = map[string]struct{}{
	"api-key":                 {},
	"apikey":                  {},
	"authorization":           {},
	"proxy-authorization":     {},
	"x-access-token":          {},
	"x-amz-security-token":    {},
	"x-api-key":               {},
	"x-apikey":                {},
	"x-auth-email":            {},
	"x-auth-key":              {},
	"x-auth-token":            {},
	"x-aws-access-token":      {},
	"x-aws-secret-access-key": {},
	"x-client-secret":         {},
	"x-csrf-token":            {},
	"x-goog-api-key":          {},
	"x-refresh-token":         {},
	"x-secret-key":            {},
	"x-token":                 {},
	"x-xsrf-token":            {},
}

func (e *Engine) record(doc *restfile.Document, req *restfile.Request, res runResult) {
	hs := e.rt.History()
	if hs == nil || req == nil {
		return
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

	secs := e.secretValues(doc, req, res.Environment, res.RuntimeSecrets...)
	mask := !req.Metadata.AllowSensitiveHeaders
	snip := "<body suppressed>"
	if !req.Metadata.NoLog {
		ct := ""
		if resp.Headers != nil {
			ct = resp.Headers.Get("Content-Type")
		}
		meta := binaryview.Analyze(resp.Body, ct)
		if meta.Kind == binaryview.KindBinary || !meta.Printable {
			snip = fmtBinary(meta, len(resp.Body))
		} else {
			snip = redactText(string(resp.Body), secs, false)
			if len(snip) > 2000 {
				snip = snip[:2000]
			}
		}
	}

	txt := redactText(res.RequestText, secs, mask)
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: res.Environment,
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.Status,
		StatusCode:  resp.StatusCode,
		Duration:    resp.Duration,
		BodySnippet: snip,
		RequestText: txt,
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
	secs := e.secretValues(doc, req, res.Environment, res.RuntimeSecrets...)
	mask := !req.Metadata.AllowSensitiveHeaders
	txt = redactText(txt, secs, mask)
	snip := strings.TrimSpace(res.SkipReason)
	if snip == "" {
		snip = "<skipped>"
	}
	if len(snip) > 2000 {
		snip = snip[:2000]
	}
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: res.Environment,
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      "SKIPPED",
		BodySnippet: snip,
		RequestText: txt,
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
	secs := e.secretValues(doc, req, res.Environment, res.RuntimeSecrets...)
	mask := !req.Metadata.AllowSensitiveHeaders
	snip := resp.Message
	if req.Metadata.NoLog {
		snip = "<body suppressed>"
	} else {
		snip = redactText(snip, secs, false)
		if len(snip) > 2000 {
			snip = snip[:2000]
		}
	}
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: res.Environment,
		RequestName: engine.ReqID(req),
		FilePath:    e.filePath(doc),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.StatusCode.String(),
		StatusCode:  int(resp.StatusCode),
		Duration:    resp.Duration,
		BodySnippet: snip,
		RequestText: redactText(res.RequestText, secs, mask),
		Description: strings.TrimSpace(req.Metadata.Description),
		Tags:        engine.Tags(req.Metadata.Tags),
	}
	_ = hs.Append(ent)
}

func (e *Engine) secretValues(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extra ...string,
) []string {
	vals := make(map[string]struct{})
	add := func(v string) {
		if strings.TrimSpace(v) == "" {
			return
		}
		vals[v] = struct{}{}
	}

	if req != nil {
		for _, v := range req.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
	}
	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
		for _, v := range doc.Globals {
			if v.Secret {
				add(v.Value)
			}
		}
	}
	if fs := e.rt.Files(); fs != nil {
		if snap := fs.Snapshot(e.envName(env), e.filePath(doc)); len(snap) > 0 {
			for _, v := range snap {
				if v.Secret {
					add(v.Value)
				}
			}
		}
	}
	if gs := e.rt.Globals(); gs != nil {
		if snap := gs.Snapshot(e.envName(env)); len(snap) > 0 {
			for _, v := range snap {
				if v.Secret {
					add(v.Value)
				}
			}
		}
	}
	for _, v := range extra {
		add(v)
	}
	if len(vals) == 0 {
		return nil
	}
	out := make([]string, 0, len(vals))
	for v := range vals {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func redactText(text string, secs []string, maskHdr bool) string {
	trim := strings.TrimSpace(text)
	if trim == "" && len(secs) == 0 {
		return text
	}
	out := text
	if len(secs) > 0 {
		for _, sec := range secs {
			if sec == "" || !strings.Contains(out, sec) {
				continue
			}
			out = strings.ReplaceAll(out, sec, "***")
		}
	}
	if !maskHdr {
		return out
	}
	ls := strings.Split(out, "\n")
	changed := false
	for i, ln := range ls {
		colon := strings.Index(ln, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(ln[:colon])
		if name == "" {
			continue
		}
		if _, ok := sensHdr[strings.ToLower(name)]; !ok {
			continue
		}
		rest := ln[colon+1:]
		padN := len(rest) - len(strings.TrimLeft(rest, " \t"))
		pad := ""
		if padN > 0 {
			pad = rest[:padN]
		}
		ls[i] = ln[:colon+1] + pad + "***"
		changed = true
	}
	if !changed {
		return out
	}
	return strings.Join(ls, "\n")
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
	Response       *httpclient.Response
	GRPC           *grpcclient.Response
	RuntimeSecrets []string
	RequestText    string
	Environment    string
	Skipped        bool
	SkipReason     string
	Executed       *restfile.Request
}
