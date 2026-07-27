package request

import (
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Execution normalizes and mutates requests, so every run starts with its own working copy.
func CloneRequest(req *restfile.Request) *restfile.Request {
	return engine.NormReq(req.Clone())
}
