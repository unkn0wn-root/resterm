package ui

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func (m *Model) historyEntriesForFileScope() []history.Entry {
	if m.historyStore == nil {
		return nil
	}
	path := strings.TrimSpace(m.historyFilePath())
	if path == "" {
		return nil
	}

	vars := historyPathVariants(path, m.workspaceRoot)
	if len(vars) == 0 {
		return nil
	}

	// One entry can match more than one path variant, so dedupe IDs
	// before sorting to keep the list stable and predictable.
	seen := make(map[string]struct{}, history.InitCap)
	out := make([]history.Entry, 0, history.InitCap)
	for _, v := range vars {
		es := m.historyStore.ByFile(v)
		for _, e := range es {
			id := strings.TrimSpace(e.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, e)
		}
	}
	if len(out) < 2 {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		return historyEntryNewerFirst(out[i], out[j])
	})
	return out
}

func historyPathVariants(path string, workspaceRoot string) []string {
	// History can contain mixed path styles from different app versions.
	// Both absolute and workspace-relative candidates are generated so lookups stay compatible.
	// Values are normalized before matching to avoid duplicates caused by path formatting.
	clean := strings.TrimSpace(path)
	if clean == "" {
		return nil
	}
	clean = filepath.Clean(clean)
	if clean == "." {
		return nil
	}
	seen := make(map[string]struct{}, 3)
	add := func(value string) {
		value = history.NormPath(value)
		if value == "" {
			return
		}
		seen[value] = struct{}{}
	}
	// Older entries can hold absolute paths while newer ones may hold
	// workspace relative paths, so both forms are queried.
	if filepath.IsAbs(clean) {
		add(clean)
		if root := strings.TrimSpace(workspaceRoot); root != "" {
			if rel, err := filepath.Rel(root, clean); err == nil {
				if rel != "." && !strings.HasPrefix(rel, "..") {
					add(rel)
				}
			}
		}
	} else {
		add(clean)
		if root := strings.TrimSpace(workspaceRoot); root != "" {
			add(filepath.Join(root, clean))
		} else if abs, err := filepath.Abs(clean); err == nil {
			add(abs)
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	return out
}
