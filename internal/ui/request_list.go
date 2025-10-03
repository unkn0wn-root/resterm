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
	badge := requestTypeBadge(i.request)
	base := i.request.Metadata.Name
	if base == "" {
		base = fmt.Sprintf("Request %d", i.index+1)
	}
	if badge == "" {
		return base
	}
	return fmt.Sprintf("%s %s", base, badge)
}

func (i requestListItem) Description() string {
	method := i.request.Method
	url := strings.TrimSpace(i.request.URL)
	if len(url) > 60 {
		url = url[:57] + "..."
	}
	return fmt.Sprintf("%s %s", method, url)
}

func (i requestListItem) FilterValue() string {
	name := i.request.Metadata.Name
	parts := []string{
		name,
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
