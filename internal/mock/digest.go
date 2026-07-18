package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// digest fingerprints the effective mock configuration so reloads can skip
// no-op handler swaps. It hashes each source path and its parsed specs, which
// automatically covers any field added to the spec later. It also hashes the
// fixture bytes each response was built from, so editing a fixture file or
// pointing a response at a different one still reloads.
func digest(docs []*restfile.Document, routes []*route) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	for _, doc := range docs {
		if len(doc.Mocks) == 0 {
			continue
		}
		_ = enc.Encode(doc.Path)
		for _, m := range doc.Mocks {
			if m != nil {
				_ = enc.Encode(m)
			}
		}
	}
	for _, rt := range routes {
		for _, v := range rt.variants {
			for _, resp := range v.responses {
				if resp.fixture != "" {
					_ = enc.Encode(struct {
						Fixture string
						Body    []byte
					}{resp.fixture, resp.body})
				}
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
