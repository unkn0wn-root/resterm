package ui

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	wsvc "github.com/unkn0wn-root/resterm/internal/workspace"
)

func listWorkspaceEntries(
	root string,
	recursive bool,
	envFile string,
	currentFile string,
	doc *restfile.Document,
) ([]files.Entry, error) {
	return wsvc.List(root, wsvc.ListOptions{
		Recursive:       recursive,
		ExplicitEnvFile: envFile,
		CurrentFile:     currentFile,
		CurrentDoc:      doc,
	})
}

func (m *Model) listWorkspaceEntries() ([]files.Entry, error) {
	return listWorkspaceEntries(
		m.ws.root,
		m.ws.recursive,
		m.ws.envFile,
		m.currentFile,
		m.doc,
	)
}

func (m *Model) syncWorkspaceEntries() ([]files.Entry, error) {
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

func (m *Model) syncWorkspaceEntriesStatus() []files.Entry {
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
