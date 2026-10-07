package intellisense

import (
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

// field holds a decoded argument and its rune offsets in the source.
type field struct {
	start, end int
	op         directive.Op
	val        int // rune offset of the value; used only when op is set
	text       string
}

func (f field) key() string {
	name, _, _ := strings.Cut(f.text, f.op.String())
	return name
}

func scanFields(text []rune) []field {
	src := string(text)
	at := func(b int) int { return utf8.RuneCountInString(src[:b]) }
	var out []field
	for token := range directive.ScanFields(src) {
		f := field{start: at(token.Start), end: at(token.End), op: token.Op, text: token.Value}
		if f.op != directive.OpNone {
			f.val = at(token.ValueStart())
		}
		out = append(out, f)
	}
	return out
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
		case f.op != directive.OpNone:
			if arg := a.findOption(f.key(), f.op); arg != nil {
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

// follow returns the arguments in effect at the caret and the fields they read.
// An argument with next takes over once the caret has left it and its value.
func (a args) follow(fields []field, caret int) (args, []field) {
	for i, s := range a.read(fields).slots {
		if fields[i].end >= caret {
			break
		}
		if s.arg != nil && s.arg.next != nil && (s.value || !s.arg.takesValue()) {
			return s.arg.next.follow(fields[i+1:], caret)
		}
	}
	return a, fields
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
