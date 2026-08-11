package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// expandStatusText resolves templates best-effort for UI display without
// executing dynamic placeholders twice.
func expandStatusText(r *vars.Resolver, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || r == nil {
		return raw
	}
	expanded, err := r.ExpandTemplatesStatic(raw)
	if err != nil {
		return raw
	}
	return strings.TrimSpace(expanded)
}

func (m *Model) statusResolver(doc *restfile.Document, req *restfile.Request) *vars.Resolver {
	rq := m.requestSvc(httpx.Options{})
	return rq.DisplayResolver(context.Background(), doc, req, m.ws.active, "", rts.Locals{})
}

func (m *Model) statusRequestLabel(doc *restfile.Document, req *restfile.Request) string {
	if req == nil {
		return ""
	}
	r := m.statusResolver(doc, req)
	name := expandStatusText(r, req.Metadata.Name)
	if name == "" {
		name = expandStatusText(r, req.URL)
	}
	return name
}

func (m *Model) statusRequestTitle(doc *restfile.Document, req *restfile.Request) string {
	if req == nil {
		return ""
	}
	r := m.statusResolver(doc, req)

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "REQ"
	}

	name := expandStatusText(r, req.Metadata.Name)
	if name == "" {
		name = expandStatusText(r, req.URL)
	}
	if len(name) > 60 {
		name = name[:57] + "..."
	}
	if name == "" {
		return method
	}
	return fmt.Sprintf("%s %s", method, name)
}
