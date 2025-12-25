package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

type themeItem struct {
	key         string
	name        string
	description string
	filterValue string
	active      bool
}

func (t themeItem) Title() string {
	if t.active {
		return fmt.Sprintf("%s (active)", t.name)
	}
	return t.name
}

func (t themeItem) Description() string {
	return t.description
}

func (t themeItem) FilterValue() string {
	return t.filterValue
}

func makeThemeItems(catalog theme.Catalog, activeKey string) []list.Item {
	defs := catalog.All()
	if len(defs) == 0 {
		return nil
	}
	items := make([]list.Item, 0, len(defs))
	normalizedActive := strings.TrimSpace(activeKey)
	for _, def := range defs {
		name := strings.TrimSpace(def.DisplayName)
		if name == "" {
			name = humaniseKey(def.Key)
		}
		var segments []string
		if desc := strings.TrimSpace(def.Metadata.Description); desc != "" {
			segments = append(segments, desc)
		}
		metaParts := make([]string, 0, 3)
		if author := strings.TrimSpace(def.Metadata.Author); author != "" {
			metaParts = append(metaParts, fmt.Sprintf("Author: %s", author))
		}
		metaParts = append(metaParts, fmt.Sprintf("Source: %s", def.Source))
		metaParts = append(metaParts, fmt.Sprintf("Key: %s", def.Key))
		segments = append(segments, strings.Join(metaParts, "  |  "))
		description := strings.Join(segments, "\n")
		filter := strings.Join(
			[]string{
				name,
				def.Key,
				def.Metadata.Author,
				def.Metadata.Description,
				string(def.Source),
			},
			" ",
		)
		items = append(items, themeItem{
			key:         def.Key,
			name:        name,
			description: description,
			filterValue: filter,
			active:      normalizedActive != "" && strings.EqualFold(def.Key, normalizedActive),
		})
	}
	return items
}

func humaniseKey(key string) string {
	if key == "" {
		return "Theme"
	}
	parts := strings.Split(key, "-")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
