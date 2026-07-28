package headless

import (
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (e *Engine) compareSpec(req *restfile.Request) *restfile.CompareSpec {
	if req == nil {
		return nil
	}
	if spec := core.BuildCompareSpec(e.cfg.CompareTargets, e.cfg.CompareBase, e.cfg.CompareGroup); spec != nil {
		return spec
	}
	return req.Metadata.Compare.Clone()
}
