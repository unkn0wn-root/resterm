package ui

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) historyEntriesForFileScope() []history.Entry {
	if m.historyStore == nil {
		return nil
	}
	path := strings.TrimSpace(m.historyFilePath())
	if path == "" {
		return nil
	}
	targets := historyPathTargets(path, m.workspaceRoot)
	reqIDs := historyRequestIdentifiers(m.doc)
	wfIDs := historyWorkflowIdentifiers(m.doc)
	entries := m.historyStore.Entries()
	matched := make([]history.Entry, 0, len(entries))
	for _, entry := range entries {
		if historyEntryMatchesFileScope(entry, targets, m.workspaceRoot, reqIDs, wfIDs) {
			matched = append(matched, entry)
		}
	}
	return matched
}

func historyPathTargets(path string, workspaceRoot string) map[string]struct{} {
	variants := historyPathVariants(path, workspaceRoot)
	if len(variants) == 0 {
		return nil
	}
	targets := make(map[string]struct{}, len(variants))
	for _, variant := range variants {
		targets[variant] = struct{}{}
	}
	return targets
}

func historyEntryMatchesFileScope(
	entry history.Entry,
	targets map[string]struct{},
	workspaceRoot string,
	reqIDs map[string]struct{},
	wfIDs map[string]struct{},
) bool {
	if historyPathMatches(entry.FilePath, targets, workspaceRoot) {
		return true
	}
	if strings.TrimSpace(entry.FilePath) != "" {
		return false
	}
	return historyEntryMatchesLegacy(entry, reqIDs, wfIDs)
}

func historyPathMatches(path string, targets map[string]struct{}, workspaceRoot string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, variant := range historyPathVariants(path, workspaceRoot) {
		if _, ok := targets[variant]; ok {
			return true
		}
	}
	return false
}

func historyPathVariants(path string, workspaceRoot string) []string {
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
		value = historyNormalizePath(value)
		if value == "" {
			return
		}
		seen[value] = struct{}{}
	}
	if filepath.IsAbs(clean) {
		add(clean)
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

func historyNormalizePath(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return ""
	}
	if runtime.GOOS == "windows" {
		clean = strings.ToLower(clean)
	}
	return clean
}

func historyEntryMatchesLegacy(
	entry history.Entry,
	reqIDs map[string]struct{},
	wfIDs map[string]struct{},
) bool {
	if entry.Method == restfile.HistoryMethodWorkflow {
		if len(wfIDs) == 0 {
			return false
		}
		name := history.NormalizeWorkflowName(entry.RequestName)
		if name == "" {
			return false
		}
		_, ok := wfIDs[name]
		return ok
	}
	if len(reqIDs) == 0 {
		return false
	}
	if name := strings.TrimSpace(entry.RequestName); name != "" {
		if _, ok := reqIDs[name]; ok {
			return true
		}
	}
	if url := strings.TrimSpace(entry.URL); url != "" {
		if _, ok := reqIDs[url]; ok {
			return true
		}
	}
	return false
}

func historyRequestIdentifiers(doc *restfile.Document) map[string]struct{} {
	if doc == nil || len(doc.Requests) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(doc.Requests)*2)
	for _, req := range doc.Requests {
		if req == nil {
			continue
		}
		if name := strings.TrimSpace(requestIdentifier(req)); name != "" {
			ids[name] = struct{}{}
		}
		if url := strings.TrimSpace(req.URL); url != "" {
			ids[url] = struct{}{}
		}
	}
	return ids
}

func historyWorkflowIdentifiers(doc *restfile.Document) map[string]struct{} {
	if doc == nil || len(doc.Workflows) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(doc.Workflows))
	for _, wf := range doc.Workflows {
		name := history.NormalizeWorkflowName(wf.Name)
		if name == "" {
			continue
		}
		ids[name] = struct{}{}
	}
	return ids
}
