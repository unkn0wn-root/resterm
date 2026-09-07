package vars

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// Template is input pre-split into literal chunks and placeholders so hot
// paths can render repeatedly without rescanning the text. It is the single
// traversal implementation behind ExpandTemplates, Render and
// ReplaceTemplateVars, so all of them agree on what counts as a placeholder.
type Template struct {
	src  string
	segs []tplSeg
}

type tplSeg struct {
	text string // literal text, or the raw {{...}} match when ph is set
	name string // trimmed placeholder name, blank for literals and {{ }}
	off  int    // byte offset in src
	ph   bool
}

func HasPlaceholder(input string) bool {
	return strings.Contains(input, "{{")
}

func CompileTemplate(input string) Template {
	ms := templateVarPattern.FindAllStringSubmatchIndex(input, -1)
	if len(ms) == 0 {
		if input == "" {
			return Template{}
		}
		return Template{src: input, segs: []tplSeg{{text: input}}}
	}

	segs := make([]tplSeg, 0, 2*len(ms)+1)
	last := 0
	for _, m := range ms {
		if m[0] > last {
			segs = append(segs, tplSeg{text: input[last:m[0]], off: last})
		}
		segs = append(segs, tplSeg{
			text: input[m[0]:m[1]],
			name: strings.TrimSpace(input[m[2]:m[3]]),
			off:  m[0],
			ph:   true,
		})
		last = m[1]
	}
	if last < len(input) {
		segs = append(segs, tplSeg{text: input[last:], off: last})
	}
	return Template{src: input, segs: segs}
}

type PlaceholderError struct {
	Match string
	Span  diag.Span
	Err   error
}

func (e *PlaceholderError) Error() string { return e.Err.Error() }
func (e *PlaceholderError) Unwrap() error { return e.Err }

// The caller sets the error class.
func (e *PlaceholderError) Diagnostic() diag.Report {
	return diag.Report{Items: []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Message:  e.Err.Error(),
		Span:     e.Span,
	}}}
}

// Render expands the template the same way ExpandTemplates does. An
// unresolvable or blank placeholder stays literal. One error is reported:
// the first structural error (cycle, depth, expression) if any occurred,
// otherwise the first undefined variable.
func (t Template) Render(r *Resolver) (string, error) {
	return t.render(r, r.exprPos, diag.Pos{}, true, true, nil)
}

func (t Template) render(
	r *Resolver,
	pos, start diag.Pos,
	allowDynamic, allowExpr bool,
	st *expandState,
) (string, error) {
	result, err := t.renderResult(r, pos, start, allowDynamic, allowExpr, st)
	return result.Value, err
}

func (t Template) renderResult(
	r *Resolver,
	pos, start diag.Pos,
	allowDynamic, allowExpr bool,
	st *expandState,
) (Expansion, error) {
	// A lenient root render (st == nil) suppresses only undefined variables,
	// which the trace records as missing. Cycles, nesting depth, and
	// expression failures still error, and nested value expansion stays
	// strict so failed values are never memoized. Structural errors outrank
	// an earlier undefined variable at every level, so a missing name in a
	// declared value can never mask a cycle or broken expression next to it.
	lenientRoot := st == nil && r.lenient
	var firstErr error
	var failed tplSeg
	var undef bool
	out := t.replace(func(seg tplSeg) string {
		if seg.name == "" {
			return seg.text
		}
		value, err := r.resolveName(seg.name, pos, allowDynamic, allowExpr, st)
		if err != nil {
			undefined := errors.Is(err, ErrUndefinedVariable)
			if undefined {
				undef = true
			}
			if lenientRoot && undefined {
				return seg.text
			}
			if replaces(err, firstErr) {
				firstErr, failed = err, seg
			}
			return seg.text
		}
		return value
	})
	// Nested values come from other declarations, so report the placeholder
	// in this template.
	if firstErr != nil && st == nil {
		firstErr = &PlaceholderError{Match: failed.text, Span: t.span(failed, start), Err: firstErr}
	}
	return Expansion{Value: out, HasUndefinedVariables: undef}, firstErr
}

func (t Template) span(seg tplSeg, start diag.Pos) diag.Span {
	switch {
	case start.Line <= 0:
		return diag.Span{}
	case start.Col <= 0:
		return diag.Span{Start: start}
	}
	from := advance(start, t.src[:seg.off])
	return diag.Span{Start: from, End: advance(from, seg.text)}
}

func advance(pos diag.Pos, text string) diag.Pos {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		pos.Line += strings.Count(text, "\n")
		pos.Col = len(text) - i
		return pos
	}
	pos.Col += len(text)
	return pos
}

// replace rebuilds the input and passes every placeholder through fn, even a
// blank {{ }}.
func (t Template) replace(fn func(seg tplSeg) string) string {
	if len(t.segs) == 0 {
		return ""
	}
	if len(t.segs) == 1 && !t.segs[0].ph {
		return t.segs[0].text
	}
	var b strings.Builder
	for _, s := range t.segs {
		if s.ph {
			b.WriteString(fn(s))
		} else {
			b.WriteString(s.text)
		}
	}
	return b.String()
}

type Unclosed struct {
	Text string
	Off  int // byte offset in the input
	Span diag.Span
}

func UnclosedPlaceholders(input string, start diag.Pos) []Unclosed {
	if !HasPlaceholder(input) {
		return nil
	}
	closed := templateVarPattern.FindAllStringIndex(input, -1)
	var out []Unclosed
	for off, c := 0, 0; ; {
		i := strings.Index(input[off:], "{{")
		if i < 0 {
			return out
		}
		i += off
		for c < len(closed) && closed[c][1] <= i {
			c++
		}
		if c < len(closed) && closed[c][0] <= i {
			off = closed[c][1]
			continue
		}
		end := i + 2 + nameLen(input[i+2:])
		if end < len(input) && input[end] == '}' {
			end++
		}
		from := advance(start, input[:i])
		out = append(out, Unclosed{
			Text: input[i:end],
			Off:  i,
			Span: diag.Span{Start: from, End: advance(from, input[i:end])},
		})
		off = end
	}
}

func nameLen(s string) int {
	i := strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '{' || r == '}' })
	if i < 0 {
		return len(s)
	}
	return i
}
