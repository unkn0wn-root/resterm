package prompt

type PathSession struct {
	gen     uint64 // Invalidates reads started before Reset.
	current pathQuery
	live    bool
	cache   map[string][]DirEntry
	pending map[string]bool
}

func (s *PathSession) Reset() {
	s.gen++
	s.live = false
	s.current = pathQuery{}
	s.cache = make(map[string][]DirEntry)
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
		s.cache = make(map[string][]DirEntry)
		s.pending = make(map[string]bool)
	}
	s.live = true
	s.current = query

	if entries, ok := s.cache[query.dir]; ok {
		return query.items(entries), DirLoad{}, true
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
	s.cache[r.Dir] = entries
	if !s.live || s.current.dir != r.Dir {
		return nil, false
	}
	return s.current.items(entries), true
}

func (s *PathSession) forget() {
	s.live = false
	s.current = pathQuery{}
}
