package ui

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	wsvc "github.com/unkn0wn-root/resterm/internal/workspace"
)

func listWorkspaceEntries(
	root string,
	recursive bool,
	envFile string,
	currentFile string,
	doc *restfile.Document,
) ([]filesvc.FileEntry, error) {
	return wsvc.List(root, wsvc.ListOptions{
		Recursive:       recursive,
		ExplicitEnvFile: envFile,
		CurrentFile:     currentFile,
		CurrentDoc:      doc,
	})
}

func (m *Model) listWorkspaceEntries() ([]filesvc.FileEntry, error) {
	return listWorkspaceEntries(
		m.ws.root,
		m.ws.recursive,
		m.ws.envFile,
		m.currentFile,
		m.doc,
	)
}

func (m *Model) syncWorkspaceEntries() ([]filesvc.FileEntry, error) {
	entries, err := m.listWorkspaceEntries()
	if err != nil {
		return nil, err
	}
	m.fileList.SetItems(makeFileItems(entries))
	if m.currentFile != "" {
		m.selectFileByPath(m.currentFile)
	}
	return entries, nil
}

func (m *Model) syncWorkspaceEntriesStatus() []filesvc.FileEntry {
	entries, err := m.syncWorkspaceEntries()
	if err != nil {
		m.setStatusMessage(statusMsg{
			text:  fmt.Sprintf("workspace error: %v", err),
			level: statusError,
		})
		return nil
	}
	return entries
}
