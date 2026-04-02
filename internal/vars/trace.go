package vars

import (
	"strings"
	"sync"
)

type ResolveTrace struct {
	Name     string
	Source   string
	Value    string
	Shadowed []string
	Uses     int
	Missing  bool
	Dynamic  bool
}

type Trace struct {
	mu    sync.Mutex
	ord   []string
	items map[string]*ResolveTrace
}

func NewTrace() *Trace {
	return &Trace{items: make(map[string]*ResolveTrace)}
}

func (t *Trace) Add(it ResolveTrace) {
	if t == nil {
		return
	}
	name := strings.TrimSpace(it.Name)
	if name == "" {
		return
	}

	key := strings.ToLower(name)

	t.mu.Lock()
	defer t.mu.Unlock()

	cur, ok := t.items[key]
	if !ok {
		if it.Uses <= 0 {
			it.Uses = 1
		}
		cp := it
		cp.Name = name
		cp.Shadowed = append([]string(nil), cp.Shadowed...)
		t.items[key] = &cp
		t.ord = append(t.ord, key)
		return
	}

	cur.Uses++
	if cur.Missing && !it.Missing {
		cur.Source = it.Source
		cur.Value = it.Value
		cur.Missing = false
		cur.Dynamic = it.Dynamic
	}
	for _, s := range it.Shadowed {
		s = strings.TrimSpace(s)
		if s == "" || containsTraceStr(cur.Shadowed, s) {
			continue
		}
		cur.Shadowed = append(cur.Shadowed, s)
	}
}

func (t *Trace) Items() []ResolveTrace {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]ResolveTrace, 0, len(t.ord))
	for _, key := range t.ord {
		it := t.items[key]
		if it == nil {
			continue
		}
		cp := *it
		cp.Shadowed = append([]string(nil), cp.Shadowed...)
		out = append(out, cp)
	}
	return out
}

func containsTraceStr(xs []string, want string) bool {
	for _, x := range xs {
		if strings.EqualFold(strings.TrimSpace(x), want) {
			return true
		}
	}
	return false
}
