package directive

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Options holds directive options keyed by their lowercased name.
type Options map[string]string

// Option names are lowercased, and a bare name means true.
// Quotes and bracketed values may contain spaces.
func ParseOptions(input string) Options {
	return parseOptions(fieldsEscaped(input))
}

// OptionFields is for callers that already separated the input. It only keeps
// key=value pairs, unlike ParseOptions where a bare key means true.
func OptionFields(fields []string) Options {
	opts := make(Options, len(fields))
	for _, field := range fields {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		opts.put(key, val)
	}
	return opts
}

func parseOptions(toks []string) Options {
	opts := make(Options, len(toks))
	for _, tok := range toks {
		key, val, ok := strings.Cut(tok, "=")
		if !ok {
			val = "true"
		}
		opts.put(key, val)
	}
	return opts
}

func (o Options) put(key, val string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	// The lexer already took the quotes off. Stripping again would eat a layer
	// from a value that is itself a quoted string.
	o[strings.ToLower(key)] = strings.TrimSpace(val)
}

func (o Options) Get(key string) string {
	return strings.TrimSpace(o[key])
}

// First returns the value of the first key that carries one. Keys present with
// an empty value are skipped so aliases can be listed in preference order.
func (o Options) First(keys ...string) (string, bool) {
	for _, key := range keys {
		if val := o.Get(key); val != "" {
			return val, true
		}
	}
	return "", false
}

func (o Options) Pop(key string) string {
	val, ok := o[key]
	if !ok {
		return ""
	}
	delete(o, key)
	return strings.TrimSpace(val)
}

// Drops every alias, not just the one it returns. Otherwise the losing spelling
// looks like an unknown option later.
func (o Options) PopAny(keys ...string) (string, bool) {
	_, val, ok := o.PopKey(keys...)
	return val, ok
}

// Reports which spelling matched, for errors that should name what the file used.
func (o Options) PopKey(keys ...string) (string, string, bool) {
	var outKey, outVal string
	for _, key := range keys {
		val, ok := o[key]
		if !ok {
			continue
		}
		if outKey == "" && strings.TrimSpace(val) != "" {
			outKey, outVal = key, strings.TrimSpace(val)
		}
		delete(o, key)
	}
	return outKey, outVal, outKey != ""
}

// A present boolean option defaults to true. Only a recognized false value
// disables it, which keeps flag-style options such as "persist" working.
// bad holds the raw text when it is not a boolean at all, so a typo gets
// reported instead of quietly switching the option on.
func (o Options) PopBool(keys ...string) (val, ok bool, bad string) {
	var (
		found bool
		raw   string
	)
	for _, key := range keys {
		got, has := o[key]
		if !has {
			continue
		}
		if !found {
			found, raw = true, got
		}
		delete(o, key)
	}
	if found {
		if strings.TrimSpace(raw) == "" {
			return true, true, ""
		}
		if parsed, valid := ParseBool(raw); valid {
			return parsed, true, ""
		}
		return true, true, raw
	}
	return false, false, ""
}

// Parsers report this as a warning. A typo should not throw away the rest of
// the directive.
type UnknownOptionsError struct {
	Directive Name
	Keys      []string
}

func (e *UnknownOptionsError) Error() string {
	quoted := make([]string, len(e.Keys))
	for i, key := range e.Keys {
		quoted[i] = strconv.Quote(key)
	}
	if len(quoted) == 1 {
		return fmt.Sprintf("unknown %s option %s", e.Directive.Tag(), quoted[0])
	}
	return fmt.Sprintf("unknown %s options %s", e.Directive.Tag(), strings.Join(quoted, ", "))
}

// nil when there is nothing to report, so callers can return it directly.
func UnknownOption(name Name, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return &UnknownOptionsError{Directive: name, Keys: keys}
}

// Whatever is left after the caller popped every key it knows about.
func (o Options) Unknown(name Name) error {
	if len(o) == 0 {
		return nil
	}
	return UnknownOption(name, slices.Sorted(maps.Keys(o))...)
}

// Profile headers use [scope] [name] followed by options. A leading option
// means the name was omitted.
type ProfileHeader struct {
	Scope   Scope
	Name    string
	Options Options
}

// ok is false only for an empty header.
func ParseProfileHeader(rest string) (ProfileHeader, bool) {
	fields := fieldsEscaped(rest)
	if len(fields) == 0 {
		return ProfileHeader{}, false
	}

	i := 0
	head := ProfileHeader{Scope: ScopeRequest}
	if scope, ok := ParseScope(fields[i]); ok {
		head.Scope = scope
		i++
	}
	if i < len(fields) && !strings.Contains(fields[i], "=") {
		head.Name = strings.TrimSpace(fields[i])
		i++
	}
	head.Options = parseOptions(fields[i:])
	return head, true
}

// A name and value may be separated by whitespace, a colon, or an equals sign.
func ParseNameValue(input string) (string, string) {
	tr := strings.TrimSpace(input)
	end := strings.IndexFunc(tr, func(r rune) bool { return !IsKeyRune(r) })
	if end == 0 {
		return "", ""
	}
	if end < 0 {
		return tr, ""
	}

	sep := tr[end:]
	val := strings.TrimLeft(sep, " \t")
	switch {
	case strings.HasPrefix(val, ":"), strings.HasPrefix(val, "="):
		val = val[1:]
	case len(val) == len(sep):
		// Neither whitespace nor a separator followed the name, so the name
		// itself holds a character that cannot appear in one.
		return "", ""
	}
	return tr[:end], strings.TrimSpace(val)
}

// CutOptions splits a directive argument into the head it starts with and the
// key=value options after it. Both halves come from the original text, so a
// quoted value keeps the spaces inside it. The span lexer escapes the same way
// ParseOptions does, or the two would disagree about where a quoted value ends.
func CutOptions(input string) (string, Options) {
	lex := &lexer{src: input, escapes: true}
	for {
		tok, ok := lex.next()
		if !ok {
			return strings.TrimSpace(input), Options{}
		}
		if isOption(input[tok.start:tok.end]) {
			return strings.TrimSpace(input[:tok.start]), ParseOptions(input[tok.start:])
		}
	}
}

// FieldSpan locates one field of an option list in its source text. Offsets are
// bytes. Eq is the equals sign that makes the field an option, -1 when the
// field is positional.
type FieldSpan struct {
	Start, End, Eq int
}

// FieldSpans reports where each field sits, scanning the way ParseOptions does,
// so a span never splits a quoted or bracketed value.
func FieldSpans(input string) []FieldSpan {
	lex := &lexer{src: input, escapes: true}
	var spans []FieldSpan
	for {
		tok, ok := lex.next()
		if !ok {
			return spans
		}
		eq := optionEq(input[tok.start:tok.end])
		if eq >= 0 {
			eq += tok.start
		}
		spans = append(spans, FieldSpan{Start: tok.start, End: tok.end, Eq: eq})
	}
}

// A name, then an equals sign typed outside quotes, then the value. This has to
// read the source. The lexer decodes "a=b" and a=b to the same text and only one
// of them was an option, so a decoded token cannot answer the question. A quote
// is not a name rune, so a quoted word never counts, and "a==b" is a comparison.
func optionEq(raw string) int {
	for i, r := range raw {
		if r == '=' {
			if i > 0 && !strings.HasPrefix(raw[i+1:], "=") {
				return i
			}
			return -1
		}
		if !IsKeyRune(r) {
			return -1
		}
	}
	return -1
}

func isOption(raw string) bool {
	return optionEq(raw) >= 0
}
