package diag

// Redact masks diagnostic text without changing the original report or its
// source excerpt. Spans refer to the caller's source and would become invalid
// if that text were rewritten.
func (r Report) Redact(mask func(string) string) Report {
	if mask == nil {
		return r
	}
	r.Items = redactEach(r.Items, func(d Diagnostic) Diagnostic {
		d.Message = mask(d.Message)
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
