package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// The encoding is compared across reloads to skip no-op handler swaps - keep it stable.
func digest(routes []*route) string {
	h := sha256.New()
	enc := json.NewEncoder(h)
	for _, rt := range routes {
		_ = enc.Encode(struct {
			Method   string
			Path     string
			Pattern  string
			Variants int
		}{
			Method:   rt.method,
			Path:     rt.path,
			Pattern:  rt.pattern,
			Variants: len(rt.variants),
		})
		for _, v := range rt.variants {
			_ = enc.Encode(struct {
				Name       string
				Default    bool
				Latency    time.Duration
				Match      restfile.MockMatch
				Status     int
				Headers    http.Header
				SourcePath string
				SourceLine int
				BodySize   int
			}{
				Name:       v.name,
				Default:    v.def,
				Latency:    v.latency,
				Match:      v.match,
				Status:     v.status,
				Headers:    v.headers,
				SourcePath: v.src.path,
				SourceLine: v.src.line,
				BodySize:   len(v.body),
			})
			_, _ = h.Write(v.body)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
