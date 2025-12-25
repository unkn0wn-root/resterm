package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

type requestDetailField struct {
	label string
	value string
	dim   bool
}

func (m *Model) openRequestDetails() {
	if m.focus != focusFile && m.focus != focusRequests {
		m.setStatusMessage(
			statusMsg{text: "Focus files or requests to view details", level: statusInfo},
		)
		return
	}
	req, doc, path := m.requestDetailContext()
	if req == nil {
		m.setStatusMessage(statusMsg{text: "Select a request to view details", level: statusInfo})
		return
	}
	m.requestDetailFields = m.buildRequestDetailFields(req, doc, path)
	m.requestDetailTitle = m.requestDetailTitleFor(req, doc)
	m.showRequestDetails = true
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	if vp := m.requestDetailViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}

func (m *Model) closeRequestDetails() {
	m.showRequestDetails = false
	m.requestDetailTitle = ""
	m.requestDetailFields = nil
	if vp := m.requestDetailViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}

func (m *Model) requestDetailContext() (*restfile.Request, *restfile.Document, string) {
	if req, doc, path := m.navigatorRequestContext(); req != nil {
		return req, doc, path
	}
	if m.currentRequest != nil {
		return m.currentRequest, m.doc, m.currentFile
	}
	return nil, nil, ""
}

func (m *Model) navigatorRequestContext() (*restfile.Request, *restfile.Document, string) {
	if m.navigator == nil {
		return nil, nil, ""
	}
	n := m.navigator.Selected()
	if n == nil || n.Kind != navigator.KindRequest {
		return nil, nil, ""
	}
	req, ok := n.Payload.Data.(*restfile.Request)
	if !ok || req == nil {
		return nil, nil, ""
	}
	path := n.Payload.FilePath
	doc := m.doc
	if path != "" && !samePath(path, m.currentFile) {
		doc = m.loadDocFor(path)
	}
	if path == "" {
		path = m.currentFile
	}
	return req, doc, path
}

func (m *Model) requestDetailTitleFor(req *restfile.Request, doc *restfile.Document) string {
	if req == nil {
		return "Request Details"
	}
	r := m.statusResolver(doc, req, m.cfg.EnvironmentName)

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "REQ"
	}

	name := expandStatusText(r, req.Metadata.Name)
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}

	target := expandStatusText(r, req.URL)
	target = strings.TrimSpace(target)
	if target == "" {
		return method
	}
	return fmt.Sprintf("%s %s", method, target)
}

func (m *Model) buildRequestDetailFields(
	req *restfile.Request,
	doc *restfile.Document,
	path string,
) []requestDetailField {
	env := m.cfg.EnvironmentName
	res := m.statusResolver(doc, req, env)
	name := expandStatusText(res, req.Metadata.Name)
	if name == "" {
		name = requestNavLabel(req, res, "Request")
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	target := expandStatusText(res, requestTarget(req))
	desc := expandStatusText(res, req.Metadata.Description)

	fields := []requestDetailField{
		{label: "Name", value: name},
		{label: "Method", value: method},
		{label: "Target", value: target},
		{label: "Desc", value: desc, dim: true},
		{label: "Tags", value: detailTags(req.Metadata.Tags)},
		{label: "Type", value: detailType(req)},
		{label: "File", value: detailFile(path, m.workspaceRoot, req.LineRange.Start)},
	}
	if meta := detailMeta(req); meta != "" {
		fields = append(fields, requestDetailField{label: "Meta", value: meta})
	}
	return filterDetailFields(fields)
}

func detailTags(tags []string) string {
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			clean = append(clean, "#"+t)
		}
	}
	return strings.Join(clean, " ")
}

func detailType(req *restfile.Request) string {
	switch {
	case req == nil:
		return ""
	case req.WebSocket != nil:
		return "WebSocket"
	case req.SSE != nil:
		return "SSE"
	case req.GRPC != nil:
		return "gRPC"
	case req.Body.GraphQL != nil:
		return "GraphQL"
	default:
		return "REST"
	}
}

func detailFile(path string, workspace string, line int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if workspace != "" {
		if rel, err := filepath.Rel(
			workspace,
			path,
		); err == nil && rel != "" &&
			!strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	path = filepath.ToSlash(path)
	if line <= 0 {
		line = 1
	}
	return fmt.Sprintf("%s:%d", path, line)
}

func detailMeta(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	var parts []string
	if req.Metadata.Auth != nil {
		typ := strings.ToUpper(strings.TrimSpace(req.Metadata.Auth.Type))
		if typ == "" {
			parts = append(parts, "Auth")
		} else {
			parts = append(parts, "Auth:"+typ)
		}
	}
	if c := len(req.Metadata.Scripts); c > 0 {
		label := "1 script"
		if c > 1 {
			label = fmt.Sprintf("%d scripts", c)
		}
		parts = append(parts, label)
	}
	if req.Metadata.Compare != nil {
		parts = append(parts, "Compare")
	}
	if req.Metadata.Trace != nil && req.Metadata.Trace.Enabled {
		parts = append(parts, "Trace")
	}
	if req.Metadata.Profile != nil {
		count := req.Metadata.Profile.Count
		if count <= 0 {
			count = 1
		}
		parts = append(parts, fmt.Sprintf("Profile x%d", count))
	}
	if req.Metadata.NoLog {
		parts = append(parts, "No log")
	}
	if req.Metadata.AllowSensitiveHeaders {
		parts = append(parts, "Allow sensitive headers")
	}
	return strings.Join(parts, " | ")
}

func filterDetailFields(fields []requestDetailField) []requestDetailField {
	var out []requestDetailField
	for _, f := range fields {
		label := strings.TrimSpace(f.label)
		val := strings.TrimSpace(f.value)
		if label == "" || val == "" {
			continue
		}
		out = append(out, requestDetailField{label: label, value: val})
	}
	return out
}

func renderDetailFields(fields []requestDetailField, width int, th theme.Theme) string {
	if width < 1 {
		width = 1
	}
	var lines []string
	for _, f := range fields {
		if line := formatDetailField(f, width, th); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func formatDetailField(f requestDetailField, width int, th theme.Theme) string {
	label := strings.TrimSpace(f.label)
	val := strings.TrimSpace(f.value)
	if label == "" || val == "" {
		return ""
	}
	labelText := th.HeaderTitle.Bold(true).Render(label + ": ")
	valueStyle := th.HeaderValue
	if f.dim {
		valueStyle = th.NavigatorSubtitle
	}
	prefix := labelText
	avail := width - visibleWidth(prefix)
	if avail < 8 {
		avail = width
	}
	segments := wrapLineSegments(valueStyle.Render(val), avail)
	indent := strings.Repeat(" ", visibleWidth(prefix))
	for i, seg := range segments {
		if i == 0 {
			segments[i] = prefix + seg
		} else {
			segments[i] = indent + seg
		}
	}
	return strings.Join(segments, "\n")
}
