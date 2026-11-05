package headless

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
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
			out = append(out, request.SecretSources{
				Doc:      doc,
				Req:      req,
				Env:      request.ResolveEnvironment(v, doc, req),
				FilePath: e.filePath(doc),
				Files:    e.rt.Files(),
				Globals:  e.rt.Globals(),
			}.Secrets()...)
		}
	}
	return out
}

func snippetHTTP(resp *httpx.Response, req *restfile.Request) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return history.BodySuppressed
	}
	return string(resp.Body)
}

func snippetGRPC(resp *grpcx.Response, req *restfile.Request) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return history.BodySuppressed
	}
	return strings.TrimSpace(resp.Message)
}
