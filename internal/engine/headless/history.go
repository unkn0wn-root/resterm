package headless

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Secrets returns the secrets of every request of doc, run or not, and the stores.
func (e *Engine) Secrets(doc *restfile.Document, env vars.Environment) []string {
	var out []string
	for _, req := range doc.Requests {
		envs := []vars.Environment{env}
		if spec := core.NormalizeCompareSpec(e.compareSpec(req)); spec != nil {
			// CheckCompare reports a target that does not resolve.
			ts, _ := e.cfg.Catalog.CompareTargets(env.Selection(), spec.Group, spec.Baseline, spec.Environments)
			for _, t := range ts {
				envs = append(envs, t.Env)
			}
		}
		for _, v := range envs {
			out = append(out, e.secretValues(doc, req, v)...)
		}
	}
	return out
}

func (e *Engine) secretValues(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	extra ...string,
) []string {
	src := request.SecretSources{
		Doc:   doc,
		Req:   req,
		Env:   request.ResolveEnvironment(env, doc, req),
		Extra: extra,
	}
	if e != nil {
		src.FilePath = e.filePath(doc)
		src.Files = e.rt.Files()
		src.Globals = e.rt.Globals()
	}
	return src.Secrets()
}

func redactText(text string, secs []string, maskHdr bool) string {
	out := text
	for _, sec := range secs {
		if sec == "" || !strings.Contains(out, sec) {
			continue
		}
		out = strings.ReplaceAll(out, sec, "***")
	}
	if !maskHdr {
		return out
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		if !header.Sensitive(line[:colon]) {
			continue
		}
		rest := line[colon+1:]
		padN := len(rest) - len(strings.TrimLeft(rest, " \t"))
		pad := ""
		if padN > 0 {
			pad = rest[:padN]
		}
		lines[i] = line[:colon+1] + pad + "***"
	}
	return strings.Join(lines, "\n")
}

func snippetHTTP(resp *httpx.Response, req *restfile.Request, secs []string) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return redactText(string(resp.Body), secs, false)
}

func snippetGRPC(resp *grpcx.Response, req *restfile.Request, secs []string) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return strings.TrimSpace(redactText(resp.Message, secs, false))
}
