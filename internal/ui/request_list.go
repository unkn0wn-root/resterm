package ui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type requestListItem struct {
	request  *restfile.Request
	index    int
	line     int
	resolver *vars.Resolver
	compact  *bool
}

func (i requestListItem) Title() string {
	if i.request == nil {
		return ""
	}
	if i.useCompact() {
		return i.compactTitle()
	}
	name := requestNavLabel(i.request, i.resolver, fmt.Sprintf("Request %d", i.index+1))
	parts := []string{name}
	if cmp := requestCompareBadge(i.request); cmp != "" {
		parts = append(parts, cmp)
	}
	if badge := requestTypeBadge(i.request); badge != "" {
		parts = append(parts, badge)
	}
	if tags := joinTags(i.request.Metadata.Tags, 3); tags != "" {
		parts = append(parts, tags)
	}
	return strings.Join(parts, " ")
}

func (i requestListItem) Description() string {
	if i.request == nil {
		return ""
	}
	return i.longDescription()
}

func (i requestListItem) useCompact() bool {
	return i.compact != nil && *i.compact
}

func (i requestListItem) compactTitle() string {
	name := requestNavLabel(i.request, i.resolver, fmt.Sprintf("Request %d", i.index+1))
	method := strings.ToUpper(strings.TrimSpace(i.request.Method))
	if method == "" {
		method = "REQ"
	}
	target := expandStatusText(i.resolver, requestTarget(i.request))
	target = truncateInline(target, 42)
	desc := condense(expandStatusText(i.resolver, i.request.Metadata.Description), 48)
	meta := compactStrings(
		requestCompareBadge(i.request),
		requestTypeBadge(i.request),
		joinTags(i.request.Metadata.Tags, 3),
	)

	info := strings.Join(compactStrings(method, target), " ")
	parts := compactStrings(name, info, desc)
	if len(meta) > 0 {
		parts = append(parts, strings.Join(meta, " "))
	}
	return strings.Join(parts, " · ")
}

func requestNavLabel(req *restfile.Request, resolver *vars.Resolver, fallback string) string {
	if req == nil {
		return strings.TrimSpace(fallback)
	}
	name := expandStatusText(resolver, req.Metadata.Name)
	if name == "" {
		name = expandStatusText(resolver, req.URL)
	}
	label := strings.TrimSpace(name)
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	return condense(label, 120)
}

func (i requestListItem) longDescription() string {
	desc := strings.TrimSpace(i.request.Metadata.Description)
	if desc != "" {
		desc = condense(desc, 80)
	}
	method := strings.ToUpper(strings.TrimSpace(i.request.Method))
	if method == "" {
		method = "REQ"
	}
	target := requestTarget(i.request)
	target = expandStatusText(i.resolver, target)
	displayTarget := truncateDisplay(target)

	if desc != "" {
		infoParts := compactStrings(method, displayTarget)
		info := strings.TrimSpace(strings.Join(infoParts, " "))
		if info == "" {
			info = method
		}
		return strings.Join(compactStrings(desc, info), "\n")
	}

	path, base := splitRequestURL(target)
	displayPath := truncateDisplay(path)
	displayBase := truncateDisplay(base)

	primary := method
	secondary := ""

	if base != "" {
		if displayPath != "" {
			primary = strings.TrimSpace(primary + " " + displayPath)
		}
		if displayBase != "" {
			secondary = displayBase
		}
	}

	if secondary == "" && displayTarget != "" {
		secondary = displayTarget
	}

	if secondary == "" {
		line := i.line
		if line <= 0 {
			line = 1
		}
		secondary = fmt.Sprintf("Line %d", line)
	}

	return strings.Join(compactStrings(primary, secondary), "\n")
}

func compactStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func truncateDisplay(value string) string {
	if len(value) > 60 {
		return value[:57] + "..."
	}
	return value
}

func truncateInline(value string, limit int) string {
	if limit <= 0 {
		return value
	}
	if len(value) > limit {
		cut := limit
		if cut > 3 {
			cut = limit - 3
		}
		if cut < 0 {
			cut = 0
		}
		return value[:cut] + "..."
	}
	return value
}

func requestTarget(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if target := strings.TrimSpace(req.URL); target != "" {
		return target
	}
	grpc := req.GRPC
	if grpc == nil {
		return ""
	}
	if target := strings.TrimSpace(grpc.FullMethod); target != "" {
		return target
	}
	service := strings.TrimSpace(grpc.Service)
	method := strings.TrimSpace(grpc.Method)
	switch {
	case service != "" && method != "":
		return fmt.Sprintf("%s.%s", service, method)
	case method != "":
		return method
	case service != "":
		return service
	}
	if target := strings.TrimSpace(grpc.Target); target != "" {
		return target
	}
	if target := strings.TrimSpace(grpc.Package); target != "" {
		return target
	}
	return ""
}

func splitRequestURL(raw string) (path string, base string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		base = u.Host
		if u.Scheme != "" {
			base = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
		}
		path = u.Path
		if u.RawPath != "" {
			path = u.RawPath
		}
		if path == "" && u.RawQuery != "" {
			path = "/"
		}
		if u.RawQuery != "" {
			path = fmt.Sprintf("%s?%s", path, u.RawQuery)
		}
		if u.Fragment != "" {
			path = fmt.Sprintf("%s#%s", path, u.Fragment)
		}
		return path, base
	}

	schemeIdx := strings.Index(raw, "://")
	if schemeIdx == -1 {
		return "", ""
	}
	remainder := raw[schemeIdx+3:]
	slashIdx := strings.Index(remainder, "/")
	if slashIdx == -1 {
		return "", raw
	}
	base = raw[:schemeIdx+3+slashIdx]
	path = remainder[slashIdx:]
	return path, base
}

func (i requestListItem) FilterValue() string {
	if i.request == nil {
		return ""
	}
	name := expandStatusText(i.resolver, i.request.Metadata.Name)
	desc := expandStatusText(i.resolver, i.request.Metadata.Description)
	target := expandStatusText(i.resolver, requestTarget(i.request))
	parts := []string{
		name,
		desc,
		strings.Join(i.request.Metadata.Tags, " "),
		requestTypeBadge(i.request), // badges are static
		i.request.Method,
		target,
	}
	return strings.Join(parts, " ")
}

func (m *Model) buildRequestItems(doc *restfile.Document) ([]requestListItem, []list.Item) {
	if doc == nil || len(doc.Requests) == 0 {
		return nil, nil
	}

	env := m.cfg.EnvironmentName
	items := make([]requestListItem, len(doc.Requests))
	listItems := make([]list.Item, len(doc.Requests))
	for idx, req := range doc.Requests {
		line := req.LineRange.Start
		if line <= 0 {
			line = 1
		}
		resolver := m.statusResolver(doc, req, env)
		item := requestListItem{
			request:  req,
			index:    idx,
			line:     line,
			resolver: resolver,
			compact:  m.reqCompact,
		}
		items[idx] = item
		listItems[idx] = item
	}
	return items, listItems
}

func requestTypeBadge(req *restfile.Request) string {
	switch {
	case req == nil:
		return ""
	case req.WebSocket != nil:
		return "[WS]"
	case req.SSE != nil:
		return "[SSE]"
	case req.GRPC != nil:
		return "[gRPC]"
	case req.Body.GraphQL != nil:
		return "[GraphQL]"
	default:
		return "[REST]"
	}
}

func requestCompareBadge(req *restfile.Request) string {
	if req == nil || req.Metadata.Compare == nil {
		return ""
	}
	return "[CMP]"
}

func joinTags(tags []string, max int) string {
	if len(tags) == 0 {
		return ""
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			clean = append(clean, t)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	shown := clean
	rem := 0
	if max > 0 && len(clean) > max {
		shown = clean[:max]
		rem = len(clean) - max
	}
	for idx, t := range shown {
		shown[idx] = "#" + t
	}
	if rem > 0 {
		shown = append(shown, fmt.Sprintf("+%d", rem))
	}
	return strings.Join(shown, " ")
}

func condense(s string, limit int) string {
	if s == "" {
		return ""
	}
	flat := strings.Join(strings.Fields(s), " ")
	if limit > 0 {
		r := []rune(flat)
		if len(r) > limit {
			cut := limit
			if cut > 3 {
				cut = limit - 3
			}
			if cut < 0 {
				cut = 0
			}
			return string(r[:cut]) + "..."
		}
	}
	return flat
}
