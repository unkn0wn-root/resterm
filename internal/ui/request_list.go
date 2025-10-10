package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type requestListItem struct {
	request *restfile.Request
	index   int
	line    int
}

func (i requestListItem) Title() string {
	if i.request == nil {
		return ""
	}
	name := strings.TrimSpace(i.request.Metadata.Name)
	if name == "" {
		name = fmt.Sprintf("Request %d", i.index+1)
	}
	parts := []string{name}
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
	desc := strings.TrimSpace(i.request.Metadata.Description)
	if desc != "" {
		desc = condense(desc, 80)
	}
	method := strings.ToUpper(strings.TrimSpace(i.request.Method))
	if method == "" {
		method = "REQ"
	}
	url := strings.TrimSpace(i.request.URL)
	if len(url) > 60 {
		url = url[:57] + "..."
	}
	info := fmt.Sprintf("%s %s", method, url)
	if desc == "" {
		return info
	}
	return strings.Join([]string{desc, info}, "\n")
}

func (i requestListItem) FilterValue() string {
	if i.request == nil {
		return ""
	}
	parts := []string{
		i.request.Metadata.Name,
		i.request.Metadata.Description,
		strings.Join(i.request.Metadata.Tags, " "),
		requestTypeBadge(i.request),
		i.request.Method,
		i.request.URL,
	}
	return strings.Join(parts, " ")
}

func buildRequestItems(
	doc *restfile.Document,
) ([]requestListItem, []list.Item) {
	if doc == nil || len(doc.Requests) == 0 {
		return nil, nil
	}
	items := make([]requestListItem, len(doc.Requests))
	listItems := make([]list.Item, len(doc.Requests))
	for idx, req := range doc.Requests {
		item := requestListItem{
			request: req,
			index:   idx,
			line:    req.LineRange.Start,
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
	case req.GRPC != nil:
		return "[gRPC]"
	case req.Body.GraphQL != nil:
		return "[GraphQL]"
	default:
		return "[REST]"
	}
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
