package prompt

import (
	"strings"
	"unicode/utf8"
)

type PathSession struct {
	gen     uint64 // Invalidates reads started before Reset.
	current pathQuery
	live    bool
	cache   map[string]*pathListing
	pending map[string]bool
}

type pathListing struct {
	entries  []DirEntry
	prepared map[PathSpec]*pathCandidates
}

type pathCandidates struct {
	all      []pathCandidate
	byPrefix map[string][]pathCandidate
}

func (s *PathSession) Reset() {
	s.gen++
	s.live = false
	s.current = pathQuery{}
	s.cache = make(map[string]*pathListing)
	s.pending = make(map[string]bool)
}

func (s *PathSession) Suggest(
	p PathProvider,
	input string,
	cursor int,
) (items []Item, load DirLoad, isPath bool) {
	request, ok := p.PathAt(input, cursor)
	if !ok {
		s.forget()
		return nil, DirLoad{}, false
	}
	query, ok := newPathQuery(request)
	if !ok {
		s.forget()
		return nil, DirLoad{}, true
	}

	if s.cache == nil {
		s.cache = make(map[string]*pathListing)
		s.pending = make(map[string]bool)
	}
	s.live = true
	s.current = query

	if listing, ok := s.cache[query.dir]; ok {
		return listing.items(query), DirLoad{}, true
	}
	if s.pending[query.dir] {
		return nil, DirLoad{}, true
	}
	s.pending[query.dir] = true
	return nil, DirLoad{Dir: query.dir, Gen: s.gen}, true
}

func (s *PathSession) Deliver(r DirRead) ([]Item, bool) {
	if r.Gen != s.gen || s.cache == nil {
		return nil, false
	}
	delete(s.pending, r.Dir)

	entries := r.Entries
	if r.Err != nil {
		entries = nil
	}
	listing := &pathListing{entries: entries}
	s.cache[r.Dir] = listing
	if !s.live || s.current.dir != r.Dir {
		return nil, false
	}
	return listing.items(s.current), true
}

func (s *PathSession) forget() {
	s.live = false
	s.current = pathQuery{}
}

func (l *pathListing) items(q pathQuery) []Item {
	if l.prepared == nil {
		l.prepared = make(map[PathSpec]*pathCandidates)
	}

	c, ok := l.prepared[q.spec]
	if !ok {
		c = &pathCandidates{
			all:      q.classify(l.entries),
			byPrefix: make(map[string][]pathCandidate),
		}
		l.prepared[q.spec] = c
	}
	return q.items(c.matching(q.prefix))
}

func (c *pathCandidates) matching(prefix string) []pathCandidate {
	if found, ok := c.byPrefix[prefix]; ok {
		return found
	}

	base := c.narrower(prefix)
	hidden := strings.HasPrefix(prefix, ".")
	out := make([]pathCandidate, 0, len(base))
	for _, candidate := range base {
		if !strings.HasPrefix(candidate.name, prefix) {
			continue
		}
		if !hidden && candidate.hidden {
			continue
		}
		out = append(out, candidate)
	}
	c.byPrefix[prefix] = out
	return out
}

// Start over when the prefix gains a leading dot because the parent result
// excludes hidden entries.
func (c *pathCandidates) narrower(prefix string) []pathCandidate {
	if prefix == "" {
		return c.all
	}
	_, size := utf8.DecodeLastRuneInString(prefix)
	parent := prefix[:len(prefix)-size]
	if strings.HasPrefix(prefix, ".") != strings.HasPrefix(parent, ".") {
		return c.all
	}
	if found, ok := c.byPrefix[parent]; ok {
		return found
	}
	return c.all
}
