package ui

import "github.com/unkn0wn-root/resterm/internal/filesvc"

func listWorkspaceEntries(
	root string,
	recursive bool,
	envFile string,
) ([]filesvc.FileEntry, error) {
	return filesvc.ListWorkspaceFiles(root, recursive, filesvc.ListOptions{
		ExplicitEnvFile: envFile,
	})
}

func (m *Model) listWorkspaceEntries() ([]filesvc.FileEntry, error) {
	return listWorkspaceEntries(m.workspaceRoot, m.workspaceRecursive, m.cfg.EnvironmentFile)
}
