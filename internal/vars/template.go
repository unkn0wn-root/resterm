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
			segs = append(segs, tplSeg{text: input[last:m[0]]})
		}
		segs = append(segs, tplSeg{
			text: input[m[0]:m[1]],
			name: strings.TrimSpace(input[m[2]:m[3]]),
			ph:   true,
		})
		last = m[1]
	}
	if last < len(input) {
		segs = append(segs, tplSeg{text: input[last:]})
	}
	return Template{src: input, segs: segs}
}

// Locator maps a one-based line and byte column of template text to its
// source position. A zero result leaves the text unlocated.
type Locator func(line, col int) diag.Pos

// at locates text written in one piece at pos.
func at(pos diag.Pos) Locator {
	return func(line, col int) diag.Pos { return posAt(pos, line, col) }
}

// posAt is line and col of text written in one piece at pos. Without a
// column, only lines are located.
func posAt(pos diag.Pos, line, col int) diag.Pos {
	switch {
	case pos.Line <= 0:
		return diag.Pos{}
	case pos.Col <= 0:
		return diag.Pos{Path: pos.Path, Line: pos.Line + line - 1}
	case line == 1:
		return diag.Pos{Path: pos.Path, Line: pos.Line, Col: pos.Col + col - 1}
	default:
		return diag.Pos{Path: pos.Path, Line: pos.Line + line - 1, Col: col}
	}
}

type PlaceholderError struct {
	Match string
	Span  diag.Span
	Err   error
}

func (e *PlaceholderError) Error() string { return e.Err.Error() }
func (e *PlaceholderError) Unwrap() error { return e.Err }

// The report the failure already carries wins, so an RTS error keeps its
// class, position, and stack. The placeholder's span fills in only when that
// report has no position. The caller sets the error class.
func (e *PlaceholderError) Diagnostic() diag.Report {
	rep := diag.ReportOf(e.Err)
	if it := &rep.Items[0]; it.Span.Start.Line <= 0 && e.Span.Start.Line > 0 {
		it.Span = e.Span
	}
	return rep
}

// Render expands the template the same way ExpandTemplates does. An
// unresolvable or blank placeholder stays literal. One error is reported:
// the first structural error (cycle, depth, expression) if any occurred,
// otherwise the first undefined variable.
func (t Template) Render(r *Resolver) (string, error) {
	return t.render(r, r.exprPos, nil, true, true, nil)
}

func (t Template) render(
	r *Resolver,
	pos diag.Pos,
	locate Locator,
	allowDynamic, allowExpr bool,
	st *expandState,
) (string, error) {
	result, err := t.renderResult(r, pos, locate, allowDynamic, allowExpr, st)
	return result.Value, err
}

func (t Template) renderResult(
	r *Resolver,
	pos diag.Pos,
	locate Locator,
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
	var failedOff int
	var undef bool
	out := t.replace(func(seg tplSeg, off int) string {
		if seg.name == "" {
			return seg.text
		}
		at := pos
		if locate != nil && seg.name[0] == '=' {
			at = t.exprPos(seg, off, pos, locate)
		}
		value, err := r.resolveName(seg.name, at, allowDynamic, allowExpr, st)
		if err != nil {
			undefined := errors.Is(err, ErrUndefinedVariable)
			if undefined {
				undef = true
			}
			if lenientRoot && undefined {
				return seg.text
			}
			if replaces(err, firstErr) {
				firstErr, failed, failedOff = err, seg, off
			}
			return seg.text
		}
		return value
	})
	// Nested values come from other declarations, so report the placeholder
	// in this template.
	if firstErr != nil && st == nil {
		firstErr = &PlaceholderError{Match: failed.text, Span: t.span(failed, failedOff, locate), Err: firstErr}
	}
	return Expansion{Value: out, HasUndefinedVariables: undef}, firstErr
}

// span locates the placeholder seg that starts at byte off of the input.
func (t Template) span(seg tplSeg, off int, locate Locator) diag.Span {
	if locate == nil {
		return diag.Span{}
	}
	start := locate(textPos(t.src[:off]))
	if start.Line <= 0 {
		return diag.Span{}
	}
	end := locate(textPos(t.src[:off+len(seg.text)]))
	if end.Line <= 0 {
		end = start
	}
	return diag.Span{Start: start, End: end}
}

// exprPos is where the expression of the {{= ...}} placeholder seg at byte
// off starts, so an evaluation error points into the file. It falls back to
// pos when the placeholder has no full position.
func (t Template) exprPos(seg tplSeg, off int, pos diag.Pos, locate Locator) diag.Pos {
	p := locate(textPos(t.src[:off+exprOffset(seg.text)]))
	if p.Line <= 0 || p.Col <= 0 {
		return pos
	}
	return p
}

// exprOffset follows the trimming resolveName applies to an {{= ...}} match.
func exprOffset(match string) int {
	inner := match[2 : len(match)-2]
	name := strings.TrimLeftFunc(inner, unicode.IsSpace)
	rest := name[1:]
	expr := strings.TrimLeftFunc(rest, unicode.IsSpace)
	return 2 + len(inner) - len(name) + 1 + len(rest) - len(expr)
}

// textPos is the one-based line and column just past text.
func textPos(text string) (line, col int) {
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		return 1 + strings.Count(text, "\n"), len(text) - i
	}
	return 1, len(text) + 1
}

// replace rebuilds the input and passes every placeholder through fn with
// its byte offset in the input, even a blank {{ }}.
func (t Template) replace(fn func(seg tplSeg, off int) string) string {
	if len(t.segs) == 0 {
		return ""
	}
	if len(t.segs) == 1 && !t.segs[0].ph {
		return t.segs[0].text
	}
	var b strings.Builder
	off := 0
	for _, s := range t.segs {
		if s.ph {
			b.WriteString(fn(s, off))
		} else {
			b.WriteString(s.text)
		}
		off += len(s.text)
	}
	return b.String()
}

type Unclosed struct {
	Text string
	Off  int // byte offset in the input
	Span diag.Span
}

// UnclosedPlaceholders runs on every parsed line, so it scans by hand
// instead of through templateVarPattern and allocates only for a finding.
func UnclosedPlaceholders(input string, start diag.Pos) []Unclosed {
	var out []Unclosed
	for off := 0; ; {
		i := strings.Index(input[off:], "{{")
		if i < 0 {
			return out
		}
		i += off
		if end, ok := placeholderEnd(input, i); ok {
			off = end
			continue
		}
		end := i + 2 + nameLen(input[i+2:])
		if end < len(input) && input[end] == '}' {
			end++
		}
		out = append(out, Unclosed{Text: input[i:end], Off: i, Span: spanIn(input, start, i, end)})
		off = end
	}
}

// placeholderEnd is where the placeholder opening at i ends, if one does.
// It mirrors templateVarPattern: at least one byte other than "}", then "}}".
func placeholderEnd(input string, i int) (int, bool) {
	j := strings.IndexByte(input[i+2:], '}')
	if j < 1 {
		return 0, false
	}
	j += i + 2
	if j+1 < len(input) && input[j+1] == '}' {
		return j + 2, true
	}
	return 0, false
}

func spanIn(input string, start diag.Pos, i, end int) diag.Span {
	line, col := textPos(input[:i])
	from := posAt(start, line, col)
	line, col = textPos(input[:end])
	return diag.Span{Start: from, End: posAt(start, line, col)}
}

func nameLen(s string) int {
	i := strings.IndexFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == '{' || r == '}' })
	if i < 0 {
		return len(s)
	}
	return i
}
