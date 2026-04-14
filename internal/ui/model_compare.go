package ui

import (
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) compareSpecForRequest(req *restfile.Request) *restfile.CompareSpec {
	if req == nil || req.Metadata.Compare == nil {
		return nil
	}
	if spec := core.BuildCompareSpec(m.cfg.CompareTargets, m.cfg.CompareBase); spec != nil {
		return spec
	}
	return core.CloneCompareSpec(req.Metadata.Compare)
}
