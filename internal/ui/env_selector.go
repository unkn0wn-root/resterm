package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type envItem struct {
	group   string
	profile string
	name    string
	active  bool
}

func (e envItem) Title() string {
	if e.group != "" {
		mark := "  "
		if e.active {
			mark = "✓ "
		}
		return mark + e.group + " = " + e.profile
	}
	if e.active {
		return "✓ " + e.name
	}
	return e.name
}

func (e envItem) Description() string {
	return ""
}

func (e envItem) FilterValue() string {
	if e.group != "" {
		return e.group + " = " + e.profile
	}
	return e.name
}

func makeEnvItems(cat vars.Catalog, sel vars.Selection) []list.Item {
	if cat.Empty() {
		return nil
	}
	if !cat.Grouped() {
		names := cat.Names()
		items := make([]list.Item, 0, len(names))
		for _, name := range names {
			items = append(items, envItem{
				name:   name,
				active: strings.EqualFold(name, sel.Name()),
			})
		}
		return items
	}

	selected := sel.Groups()
	var items []list.Item
	for _, group := range cat.Groups() {
		for _, profile := range group.ProfileNames() {
			items = append(items, envItem{
				group:   group.Name,
				profile: profile,
				active:  strings.EqualFold(selected[group.Name], profile),
			})
		}
	}
	return items
}
