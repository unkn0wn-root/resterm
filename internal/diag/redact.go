package diag

import "strings"

// Redact masks diagnostic text and source excerpts in a copy of the report.
// Source lines may contain secrets outside the failing span.
func (r Report) Redact(mask func(string) string) Report {
	if mask == nil {
		return r
	}
	src := r.Source
	r.Source = redactSource(src, mask)
	r.Items = redactEach(r.Items, func(d Diagnostic) Diagnostic {
		d.SourceLine, d.SourceCol = maskedPos(sourceFor(d, src), excerptPos(d), mask)
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

func maskedPos(src []byte, pos Pos, mask func(string) string) (int, int) {
	lines := sourceLines(src)
	if pos.Line <= 0 || pos.Line > len(lines) {
		return 0, 0
	}
	before := min(max(pos.Col-1, 0), len(lines[pos.Line-1]))
	for _, line := range lines[:pos.Line-1] {
		before += len(line) + 1
	}
	// Mask the raw prefix so multiline secrets match as they do in redactSource.
	masked := mask(string(src[:before]))
	return strings.Count(masked, "\n") + 1, len(masked) - strings.LastIndex(masked, "\n")
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
