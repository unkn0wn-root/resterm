package history

import (
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/redact"
)

// Buffer keeps raw body snippets up to this many bytes in all to mask them again, so a session's memory stays bounded.
const remaskBudget = 16 << 20

type held struct {
	Entry
	size int
	// The store shows a body this copy no longer holds, so the next new secret masks it whole.
	dropped bool
}

// Buffer is a Store that keeps appended entries until Flush, so each is written
// masked with the secrets learned by then.
type Buffer struct {
	Store

	// mu guards the written entries and serializes Flush and Delete. Lock order: mu, then smu.
	mu      sync.Mutex
	entries []held
	oldest  int // entries[:oldest] hold no snippet bytes
	total   int // snippet bytes entries hold

	// smu is never held while masking or writing, so Append, Learn and Secrets do not wait for a Flush.
	smu   sync.Mutex
	known map[string]struct{}
	sec   redact.Set
	fresh []string // learned, not yet probed for
	queue []Entry  // appended, not yet taken by a Flush
}

// Append holds e until the next Flush and always returns nil.
func (b *Buffer) Append(e Entry) error {
	b.smu.Lock()
	defer b.smu.Unlock()
	b.queue = append(b.queue, e)
	return nil
}

// Learn adds secrets the next Flush masks with.
func (b *Buffer) Learn(secrets ...string) {
	b.smu.Lock()
	defer b.smu.Unlock()
	n := len(b.fresh)
	for _, s := range secrets {
		if _, ok := b.known[s]; ok || s == "" {
			continue
		}
		if b.known == nil {
			b.known = make(map[string]struct{})
		}
		b.known[s] = struct{}{}
		b.fresh = append(b.fresh, s)
	}
	if len(b.fresh) > n {
		b.sec = redact.New(slices.Collect(maps.Keys(b.known))...)
	}
}

// Delete also forgets the entry, so no later Flush writes it again.
func (b *Buffer) Delete(id string) (bool, error) {
	b.mu.Lock()
	if i := slices.IndexFunc(b.entries, func(h held) bool { return h.ID == id }); i >= 0 {
		b.total -= b.entries[i].size
		b.entries = slices.Delete(b.entries, i, i+1)
		if i < b.oldest {
			b.oldest--
		}
	}
	b.smu.Lock()
	b.queue = slices.DeleteFunc(b.queue, func(e Entry) bool { return e.ID == id })
	b.smu.Unlock()
	b.mu.Unlock()
	return b.Store.Delete(id)
}

// Flush learns secrets, then writes the buffered entries masked with every secret learned so far.
// A secret learned since the last Flush masks the entries it already wrote again.
func (b *Buffer) Flush(secrets ...string) error {
	b.Learn(secrets...)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.smu.Lock()
	sec, fresh, queue := b.sec, b.fresh, b.queue
	b.fresh, b.queue = nil, nil
	b.smu.Unlock()

	var errs []error
	if len(fresh) > 0 {
		// Only an entry a new secret matches can hold one, so one probe pass finds them all.
		probe := redact.New(fresh...)
		for i := range b.entries {
			h := &b.entries[i]
			if !h.dropped {
				if _, hit := h.masked(probe); !hit {
					continue
				}
			}
			e, _ := h.masked(sec)
			if err := b.Store.Append(e); err != nil {
				errs = append(errs, err)
				continue
			}
			h.dropped = false
		}
		if len(errs) > 0 {
			// A row the pass failed to rewrite may still hold these secrets, so the next Flush probes for them again.
			b.smu.Lock()
			b.fresh = append(b.fresh, fresh...)
			b.smu.Unlock()
		}
	}

	drop := func(h *held) {
		if h.size == 0 {
			return
		}
		b.total -= h.size
		h.size, h.dropped = 0, true
		if h.BodySnippet != "" {
			h.BodySnippet = redact.Mask
		}
		if c := h.Compare; c != nil {
			cc := *c
			cc.Results = slices.Clone(c.Results)
			for i := range cc.Results {
				if cc.Results[i].BodySnippet != "" {
					cc.Results[i].BodySnippet = redact.Mask
				}
			}
			h.Compare = &cc
		}
	}
	for _, e := range queue {
		m, _ := e.masked(sec)
		if err := b.Store.Append(m); err != nil {
			// Not retried, so a store that stays down holds no raw bodies.
			errs = append(errs, err)
			continue
		}
		h := held{Entry: e, size: len(e.BodySnippet)}
		if e.Compare != nil {
			for _, r := range e.Compare.Results {
				h.size += len(r.BodySnippet)
			}
		}
		b.total += h.size
		// A body over the budget on its own is never held, so no older entry gives way to it.
		if h.size > remaskBudget {
			drop(&h)
		}
		b.entries = append(b.entries, h)
		for b.total > remaskBudget {
			drop(&b.entries[b.oldest])
			b.oldest++
		}
	}
	return errors.Join(errs...)
}

// Secrets returns the Set Flush masks entries with, so lookups can mask their keys the same way.
func (b *Buffer) Secrets() redact.Set {
	b.smu.Lock()
	defer b.smu.Unlock()
	return b.sec
}
