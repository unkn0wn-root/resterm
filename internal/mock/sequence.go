package mock

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const maxSequenceKeyBytes = 4 << 10

// sequenceCursor tracks the next response step per sequence key. Unkeyed
// sequences share the "" key and never count against the key limit.
type sequenceCursor struct {
	mu    sync.Mutex
	steps map[string]uint64
	limit int
}

func (c *sequenceCursor) advance(key string, last int) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, exists := c.steps[key]
	if !exists && key != "" && len(c.steps) >= c.limit {
		return 0, false
	}
	if c.steps == nil {
		c.steps = make(map[string]uint64)
	}
	c.steps[key] = min(next+1, uint64(last))
	return int(min(next, uint64(last))), true
}

func (c *sequenceCursor) reset() {
	c.mu.Lock()
	clear(c.steps)
	c.mu.Unlock()
}

func (c *sequenceCursor) setLimit(limit int) {
	c.mu.Lock()
	c.limit = limit
	c.mu.Unlock()
}

func (v *variant) sequenceKey(p *probe) (string, *problem) {
	key := v.sequenceKeySpec
	if key.IsZero() {
		return "", nil
	}

	var value string
	switch key.Source {
	case restfile.MockSequenceKeySourcePath:
		wildcard := v.pathParams[key.Name]
		value = p.r.PathValue(wildcard)
	case restfile.MockSequenceKeySourceQuery:
		if values := p.query()[key.Name]; len(values) > 0 {
			value = values[0]
		}
	case restfile.MockSequenceKeySourceHeader:
		if values := headerValues(p.r, key.Name); len(values) > 0 {
			value = values[0]
		}
	case restfile.MockSequenceKeySourceCookie:
		if cookies := p.r.CookiesNamed(key.Name); len(cookies) > 0 {
			value = cookies[0].Value
		}
	}
	if value == "" {
		return "", &problem{
			status: http.StatusBadRequest,
			detail: fmt.Sprintf("mock sequence key %s is missing or empty", key.String()),
		}
	}
	if len(value) > maxSequenceKeyBytes {
		return "", &problem{
			status: http.StatusBadRequest,
			detail: fmt.Sprintf("mock sequence key %s exceeds the 4 KiB limit", key.String()),
		}
	}
	return value, nil
}
