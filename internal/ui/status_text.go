package ui

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Keep room for run counters and result markers.
const statusRunLabelMax = 24

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

// statusRunLabel returns a short request name for the status bar.
func statusRunLabel(r *vars.Resolver, req *restfile.Request) string {
	if req == nil {
		return ""
	}
	name := expandStatusText(r, req.Metadata.Name)
	if name == "" {
		name = shortURLLabel(expandStatusText(r, req.URL))
	}
	return truncateToWidth(name, statusRunLabelMax)
}

func shortURLLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	for _, part := range []string{u.Path, u.Opaque} {
		if seg := path.Base(strings.TrimRight(part, "/")); seg != "" && seg != "." && seg != "/" {
			return seg
		}
	}
	if u.Host != "" {
		return u.Host
	}
	return raw
}

func (m *Model) statusRequestTitle(doc *restfile.Document, req *restfile.Request) string {
	return requestTitle(m.statusResolver(doc, req), req)
}

// statusRunTitles returns full and status-bar labels using a shared resolver.
func (m *Model) statusRunTitles(
	doc *restfile.Document,
	req *restfile.Request,
) (title, short string) {
	r := m.statusResolver(doc, req)
	title = requestTitle(r, req)
	if title == "" {
		title = requestBaseTitle(req)
	}
	short = statusRunLabel(r, req)
	if short == "" {
		short = title
	}
	return title, short
}

func requestTitle(r *vars.Resolver, req *restfile.Request) string {
	if req == nil {
		return ""
	}

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
