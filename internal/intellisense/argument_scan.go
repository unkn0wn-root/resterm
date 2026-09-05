package intellisense

import (
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

// field holds a decoded argument and its rune offsets in the source.
type field struct {
	start, end int
	form       argForm
	eq         int // rune offset of '=', or -1 for a positional field
	text       string
}

func (f field) key() string {
	name, _, _ := strings.Cut(f.text, "=")
	return strings.TrimSuffix(name, "<")
}

func scanFields(text []rune) []field {
	src := string(text)
	at := func(b int) int { return utf8.RuneCountInString(src[:b]) }
	var out []field
	for token := range directive.ScanFields(src) {
		f := field{start: at(token.Start), end: at(token.End), eq: -1, text: token.Value}
		if token.Eq >= 0 {
			f.form, f.eq = formOption, at(token.Eq)
		} else if eq := budgetEq(src[token.Start:token.End]); eq >= 0 {
			f.form, f.eq = formBudget, at(token.Start+eq)
		}
		out = append(out, f)
	}
	return out
}

// budgetEq returns the byte offset of '=' in "key<=value", or -1 for other fields.
func budgetEq(raw string) int {
	key, _, ok := strings.Cut(raw, "<=")
	if !ok || key == "" || strings.ContainsFunc(key, func(r rune) bool { return !directive.IsKeyRune(r) }) {
		return -1
	}
	return len(key) + 1
}

type slot struct {
	arg   *argument
	value bool // the field holds the argument's value rather than its name
}

type scan struct {
	slots []slot
	open  *argument // argument waiting for a value in the next field
	taken bool      // the directive's positional value is present
}

func (a args) read(fields []field) scan {
	out := scan{slots: make([]slot, len(fields))}
	for i, f := range fields {
		switch {
		case out.open != nil:
			out.slots[i] = slot{arg: out.open, value: true}
			out.open = nil
		case f.eq >= 0:
			if arg := a.findOption(f.key(), f.form); arg != nil {
				out.slots[i] = slot{arg: arg, value: true}
			}
		default:
			if arg := a.findWord(f.text); arg != nil {
				out.slots[i] = slot{arg: arg}
				if arg.takesValue() && !arg.loadsLine() {
					out.open = arg
				}
				continue
			}
			if a.value != nil && !a.value.loadsLine() && (!out.taken || a.value.repeat) {
				out.slots[i] = slot{arg: a.value, value: true}
				out.taken = true
			}
		}
	}
	return out
}

// loadValue finds a path that uses the rest of the line and its starting rune offset.
func (a args) loadValue(fields []field, text []rune) (*argument, int, bool) {
	if a.value != nil && a.value.loadsLine() {
		start, ok := loadStart(text, 0, a.value.value.path.form)
		return a.value, start, ok
	}
	for _, f := range fields {
		arg := a.findWord(f.text)
		if arg == nil || !arg.loadsLine() {
			continue
		}
		start, ok := loadStart(text, f.end, arg.value.path.form)
		// Start value completion only after the caret leaves the argument name.
		return arg, max(start, f.end+1), ok
	}
	return nil, 0, false
}

func loadStart(text []rune, from int, form pathForm) (int, bool) {
	i := skipSpace(text, from)
	if i < len(text) && text[i] == '<' {
		return skipSpace(text, i+1), true
	}
	return i, form != pathLoadOnly
}
