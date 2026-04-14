package request

import (
	"github.com/unkn0wn-root/resterm/internal/registry"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (e *Engine) registryIndex() *registry.Index {
	if e == nil {
		return nil
	}
	if e.cfg.Registry != nil {
		e.rg = e.cfg.Registry
		return e.rg
	}
	if e.rg == nil {
		e.rg = registry.New()
	}
	if !e.rg.Match(e.cfg.WorkspaceRoot, e.cfg.Recursive) {
		e.rg.Load(e.cfg.WorkspaceRoot, e.cfg.Recursive)
	}
	return e.rg
}

func (e *Engine) syncRegistry(doc *restfile.Document) {
	if e == nil || doc == nil {
		return
	}
	if ix := e.registryIndex(); ix != nil {
		ix.Sync(doc)
	}
}
