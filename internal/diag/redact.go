package diag

// Redact masks diagnostic text and source excerpts in a copy of the report.
// Source lines may contain secrets outside the failing span.
func (r Report) Redact(mask func(string) string) Report {
	if mask == nil {
		return r
	}
	r.Source = redactSource(r.Source, mask)
	r.Items = redactEach(r.Items, func(d Diagnostic) Diagnostic {
		d.Message = mask(d.Message)
		d.Source = redactSource(d.Source, mask)
		d.Span = redactSpan(d.Span, mask)
		d.Labels = redactEach(d.Labels, func(l Label) Label {
			l.Message = mask(l.Message)
			l.Span = redactSpan(l.Span, mask)
			return l
		})
		d.Notes = redactEach(d.Notes, func(n Note) Note {
			n.Message = mask(n.Message)
			n.Span = redactSpan(n.Span, mask)
			return n
		})
		d.Chain = redactChain(d.Chain, mask)
		d.Frames = redactEach(d.Frames, func(f StackFrame) StackFrame {
			f.Name = mask(f.Name)
			return f
		})
		return d
	})
	return r
}

func redactChain(src []ChainEntry, mask func(string) string) []ChainEntry {
	return redactEach(src, func(e ChainEntry) ChainEntry {
		e.Message = mask(e.Message)
		e.Children = redactChain(e.Children, mask)
		return e
	})
}

func redactSource(src []byte, mask func(string) string) []byte {
	if len(src) == 0 {
		return src
	}
	return []byte(mask(string(src)))
}

func redactSpan(s Span, mask func(string) string) Span {
	s.Label = mask(s.Label)
	return s
}

func redactEach[T any](src []T, fn func(T) T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	for i, v := range src {
		out[i] = fn(v)
	}
	return out
}
