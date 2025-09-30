package ui

import (
	"sort"

	"github.com/charmbracelet/bubbles/list"
)

type envItem struct {
	name string
}

func (e envItem) Title() string {
	return e.name
}

func (e envItem) Description() string {
	return ""
}

func (e envItem) FilterValue() string {
	return e.name
}

func makeEnvItems(envs map[string]map[string]string) []list.Item {
	if len(envs) == 0 {
		return nil
	}
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		items = append(items, envItem{name: name})
	}
	return items
}
