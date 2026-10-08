package directive

import (
	"iter"
	"strings"
)

// Op is the operator between an option's name and its value.
type Op uint8

const (
	OpNone Op = iota // positional field
	OpEq             // key=value
	OpLe             // key<=value (@trace budget)
)

// Match longer operators first when one is a prefix of another.
var ops = []Op{OpLe, OpEq}

func (o Op) String() string {
	switch o {
	case OpEq:
		return "="
	case OpLe:
		return "<="
	}
	return ""
}

// FieldSpan locates a field in the source. All offsets are in bytes.
// At marks the operator and is only meaningful when Op != OpNone.
type FieldSpan struct {
	Start, End int
	Op         Op
	At         int
}

func (s FieldSpan) ValueStart() int {
	return s.At + len(s.Op.String())
}

// Field pairs a decoded option field with its byte offsets in the source.
type Field struct {
	FieldSpan
	Value string
}

// Positional reports whether f is a plain value. Fields starting with an
// operator (such as =x) are invalid options and return false.
func (f Field) Positional() bool {
	return f.Op == OpNone && !noKey(f.Value)
}

// ScanFields yields the values returned by Fields with their source byte offsets.
// It accepts incomplete quotes, JSON, and calls, and preserves bare backslashes.
func ScanFields(input string) iter.Seq[Field] {
	return scanFields(input, false)
}

func scanFields(input string, escapes bool) iter.Seq[Field] {
	return func(yield func(Field) bool) {
		lex := &lexer{src: input, escapes: escapes}
		for {
			tok, ok := lex.next()
			if !ok {
				return
			}
			raw := input[tok.start:tok.end]
			span := FieldSpan{Start: tok.start, End: tok.end}
			if op, at := scanOp(raw); op != OpNone {
				span.Op, span.At = op, tok.start+at
			}
			if !yield(Field{FieldSpan: span, Value: tok.val}) {
				return
			}
		}
	}
}

// FieldSpans uses ParseOptions' quote and escape rules; quoted and bracketed
// values stay in one span.
func FieldSpans(input string) []FieldSpan {
	var spans []FieldSpan
	for field := range scanFields(input, true) {
		spans = append(spans, field.FieldSpan)
	}
	return spans
}

// Read the raw field: "a=b" and a=b decode to the same text, but only a=b
// is an option. A quoted key or a comparison such as a==b has no option operator.
func scanOp(raw string) (Op, int) {
	i := strings.IndexFunc(raw, func(r rune) bool { return !IsKeyRune(r) })
	if i <= 0 {
		return OpNone, 0
	}
	for _, op := range ops {
		if !strings.HasPrefix(raw[i:], op.String()) {
			continue
		}
		if op == OpEq && strings.HasPrefix(raw[i+1:], "=") {
			return OpNone, 0
		}
		return op, i
	}
	return OpNone, 0
}

func isOption(raw string) bool {
	op, _ := scanOp(raw)
	return op == OpEq
}
