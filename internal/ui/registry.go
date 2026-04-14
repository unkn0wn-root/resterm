package ui

import (
	"github.com/unkn0wn-root/resterm/internal/registry"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) registryIndex() *registry.Index {
	if m == nil {
		return nil
	}
	if m.rg == nil {
		m.rg = registry.New()
	}
	if !m.rg.Match(m.workspaceRoot, m.workspaceRecursive) {
		m.rg.Load(m.workspaceRoot, m.workspaceRecursive)
	}
	return m.rg
}

func (m *Model) syncRegistry(doc *restfile.Document) {
	if m == nil || doc == nil {
		return
	}
	if ix := m.registryIndex(); ix != nil {
		ix.Sync(doc)
	}
}
