package runner

import (
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func cloneDoc(src *restfile.Document) *restfile.Document {
	doc := src.Clone()
	for _, req := range doc.Requests {
		engine.NormReq(req)
	}
	return doc
}
