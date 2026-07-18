package vars

import (
	"regexp"
	"strings"
)

var templateVarPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// Template is input pre-split into literal chunks and placeholders so hot
// paths can render repeatedly without rescanning the text. It is the single
// traversal implementation behind ExpandTemplates, Render and
// ReplaceTemplateVars, so all of them agree on what counts as a placeholder.
type Template struct {
	segs []tplSeg
}

type tplSeg struct {
	text string // literal text, or the raw {{...}} match when ph is set
	name string // trimmed placeholder name, blank for literals and {{ }}
	ph   bool
}

func CompileTemplate(input string) Template {
	ms := templateVarPattern.FindAllStringSubmatchIndex(input, -1)
	if len(ms) == 0 {
		if input == "" {
			return Template{}
		}
		return Template{segs: []tplSeg{{text: input}}}
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
	return Template{segs: segs}
}

// Render expands the template the same way ExpandTemplates does. An
// unresolvable or blank placeholder stays literal and only the first error
// is reported.
func (t Template) Render(r *Resolver) (string, error) {
	return t.render(r, r.exprPos, true, true)
}

func (t Template) render(r *Resolver, pos ExprPos, allowDynamic, allowExpr bool) (string, error) {
	var firstErr error
	out := t.replace(func(match, name string) string {
		if name == "" {
			return match
		}
		value, err := r.resolveName(name, pos, allowDynamic, allowExpr)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return match
		}
		return value
	})
	return out, firstErr
}

// replace rebuilds the input and passes every placeholder through fn, even a
// blank {{ }}.
func (t Template) replace(fn func(match, name string) string) string {
	if len(t.segs) == 0 {
		return ""
	}
	if len(t.segs) == 1 && !t.segs[0].ph {
		return t.segs[0].text
	}
	var b strings.Builder
	for _, s := range t.segs {
		if s.ph {
			b.WriteString(fn(s.text, s.name))
		} else {
			b.WriteString(s.text)
		}
	}
	return b.String()
}
