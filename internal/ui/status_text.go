package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
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

func (m *Model) statusResolver(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extras ...map[string]string,
) *vars.Resolver {
	base := m.rtsBase(doc, "")
	return m.buildDisplayResolver(context.Background(), doc, req, env, base, nil, extras...)
}

func (m *Model) statusRequestTarget(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extras ...map[string]string,
) string {
	if req == nil {
		return ""
	}
	r := m.statusResolver(doc, req, env, extras...)
	target := expandStatusText(r, req.URL)
	if target == "" {
		target = strings.TrimSpace(req.URL)
	}
	return target
}

func (m *Model) statusRequestTitle(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extras ...map[string]string,
) string {
	if req == nil {
		return ""
	}
	r := m.statusResolver(doc, req, env, extras...)

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
