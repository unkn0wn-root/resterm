package ui

import (
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) compareSpecForRequest(req *restfile.Request) *restfile.CompareSpec {
	if req == nil || req.Metadata.Compare == nil {
		return nil
	}
	if spec := core.BuildCompareSpec(m.cfg.Compare); spec != nil {
		return spec
	}
	return req.Metadata.Compare.Clone()
}
