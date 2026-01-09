package ui

import (
	"path/filepath"
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
	clean := strings.TrimSpace(path)
	if clean == "" {
		return nil
	}
	clean = filepath.Clean(clean)
	if clean == "." {
		return nil
	}
	targets := map[string]struct{}{clean: {}}
	if filepath.IsAbs(clean) {
		return targets
	}
	if workspaceRoot != "" {
		targets[filepath.Clean(filepath.Join(workspaceRoot, clean))] = struct{}{}
		return targets
	}
	if abs, err := filepath.Abs(clean); err == nil {
		targets[filepath.Clean(abs)] = struct{}{}
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
	clean := strings.TrimSpace(path)
	if clean == "" {
		return false
	}
	clean = filepath.Clean(clean)
	if clean == "." {
		return false
	}
	if _, ok := targets[clean]; ok {
		return true
	}
	if filepath.IsAbs(clean) {
		return false
	}
	if workspaceRoot != "" {
		joined := filepath.Clean(filepath.Join(workspaceRoot, clean))
		_, ok := targets[joined]
		return ok
	}
	if abs, err := filepath.Abs(clean); err == nil {
		_, ok := targets[filepath.Clean(abs)]
		return ok
	}
	return false
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
