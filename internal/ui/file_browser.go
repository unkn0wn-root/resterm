package ui

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
)

type fileItem struct {
	entry filesvc.FileEntry
}

func (f fileItem) Title() string {
	return f.entry.Name
}

func (f fileItem) Description() string {
	return filepath.Dir(f.entry.Name)
}

func (f fileItem) FilterValue() string {
	return f.entry.Name
}

func makeFileItems(entries []filesvc.FileEntry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = fileItem{entry: e}
	}
	return items
}

func selectedFilePath(it list.Item) string {
	if it == nil {
		return ""
	}
	if fi, ok := it.(fileItem); ok {
		return fi.entry.Path
	}
	return ""
}
