package directive

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Options stores parsed directive options and tracks conflicting aliases.
type Options struct {
	vals  map[string]string
	clash map[string][]string
}

func newOptions(size int) Options {
	return Options{vals: make(map[string]string, size), clash: map[string][]string{}}
}

func (o Options) Len() int {
	return len(o.vals)
}

func (o Options) Lookup(key string) (string, bool) {
	val, ok := o.vals[key]
	return val, ok
}

func (o Options) Has(key string) bool {
	_, ok := o.vals[key]
	return ok
}

func (o Options) Keys() []string {
	return slices.Sorted(maps.Keys(o.vals))
}

func (o Options) All() iter.Seq2[string, string] {
	return maps.All(o.vals)
}

func (o Options) CopyTo(dst map[string]string) {
	maps.Copy(dst, o.vals)
}

// Option names are lowercased, and a bare name means true.
// Quotes and bracketed values may contain spaces.
// Repeated options are reported as errors.
func ParseOptions(name Name, input string) (Options, error) {
	return parseOptions(name, fieldsEscaped(input))
}

// OptionsOpen reports whether a quoted or bracketed value continues past input.
func OptionsOpen(input string) bool {
	lex := &lexer{src: input, escapes: true}
	open := false
	for _, ok := lex.next(); ok; _, ok = lex.next() {
		open = len(lex.closers) > 0 || lex.quote != 0
	}
	return open
}

// OptionFields is for callers that already separated the input. It only keeps
// key=value pairs, unlike ParseOptions where a bare key means true.
func OptionFields(name Name, fields []string) (Options, error) {
	return collectOptions(name, fields, false)
}

func parseOptions(name Name, toks []string) (Options, error) {
	return collectOptions(name, toks, true)
}

func collectOptions(name Name, toks []string, bareIsTrue bool) (Options, error) {
	opts := newOptions(len(toks))
	var rep repeats
	for _, tok := range toks {
		key, val, ok := strings.Cut(tok, "=")
		if !ok {
			if !bareIsTrue {
				continue
			}
			val = "true"
		}
		rep.add(opts.put(key, val))
	}
	return opts, rep.err(name)
}

// Every option is visited even after one fails, so a line with two mistakes
// reports both.
func ApplyOptions(name Name, raw string, aliases [][]string, apply func(key, val string) error) error {
	opts, err := ParseOptions(name, raw)
	errs := []error{err}
	for _, group := range aliases {
		opts.Aliases(group...)
	}
	for _, key := range opts.Keys() {
		errs = append(errs, apply(key, opts.Get(key)))
	}
	return errors.Join(append(errs, opts.Conflicts(name))...)
}

// The only writer, so every stored key is lowercase and every value trimmed.
// Readers below rely on that.
func (o Options) put(key, val string) (repeated string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	_, seen := o.vals[key]
	// The lexer already took the quotes off. Stripping again would eat a layer
	// from a value that is itself a quoted string.
	o.vals[key] = strings.TrimSpace(val)
	if !seen {
		return ""
	}
	return key
}

type repeats []string

func (r *repeats) add(key string) {
	if key != "" && !slices.Contains(*r, key) {
		*r = append(*r, key)
	}
}

func (r *repeats) err(name Name) error {
	slices.Sort(*r)
	return RepeatedOption(name, *r...)
}

func (o Options) Get(key string) string {
	return o.vals[key]
}

// First returns the value of the first key that carries one. Keys present with
// an empty value are skipped so aliases can be listed in preference order.
func (o Options) First(keys ...string) (string, bool) {
	o.given(keys)
	for _, key := range keys {
		if val := o.Get(key); val != "" {
			return val, true
		}
	}
	return "", false
}

func (o Options) Aliases(keys ...string) {
	o.given(keys)
}

func (o Options) Pop(key string) string {
	val := o.vals[key]
	delete(o.vals, key)
	return val
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
	for _, key := range o.given(keys) {
		if val := o.vals[key]; outKey == "" && val != "" {
			outKey, outVal = key, val
		}
		delete(o.vals, key)
	}
	return outKey, outVal, outKey != ""
}

func (o Options) given(keys []string) []string {
	var (
		written []string
		set     []string
	)
	for _, key := range keys {
		val, ok := o.vals[key]
		if !ok {
			continue
		}
		written = append(written, key)
		if val != "" {
			set = append(set, key)
		}
	}
	o.noteClash(set)
	return written
}

// Boolean aliases conflict whenever more than one spelling is present. An empty
// value still enables a switch, so it cannot be ignored in favor of another
// alias with an explicit value.
func (o Options) present(keys []string) []string {
	var written []string
	for _, key := range keys {
		if o.Has(key) {
			written = append(written, key)
		}
	}
	o.noteClash(written)
	return written
}

func (o Options) noteClash(keys []string) {
	if len(keys) < 2 {
		return
	}
	set := slices.Clone(keys)
	slices.Sort(set)
	o.clash[strings.Join(set, " ")] = set
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
	for _, key := range o.present(keys) {
		if !found {
			found, raw = true, o.vals[key]
		}
		delete(o.vals, key)
	}
	if found {
		if raw == "" {
			return true, true, ""
		}
		if parsed, valid := ParseBool(raw); valid {
			return parsed, true, ""
		}
		return true, true, raw
	}
	return false, false, ""
}

func quoteKeys(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = strconv.Quote(key)
	}
	return strings.Join(quoted, ", ")
}

// Parsers report this as a warning. A typo should not throw away the rest of
// the directive.
type UnknownOptionsError struct {
	Directive Name
	Keys      []string
}

func (e *UnknownOptionsError) Error() string {
	if len(e.Keys) == 1 {
		return fmt.Sprintf("unknown %s option %s", e.Directive.Tag(), quoteKeys(e.Keys))
	}
	return fmt.Sprintf("unknown %s options %s", e.Directive.Tag(), quoteKeys(e.Keys))
}

func UnknownOption(name Name, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return &UnknownOptionsError{Directive: name, Keys: keys}
}

type RepeatedOptionsError struct {
	Directive Name
	Keys      []string
}

func (e *RepeatedOptionsError) Error() string {
	if len(e.Keys) == 1 {
		return fmt.Sprintf("%s option %s is repeated", e.Directive.Tag(), quoteKeys(e.Keys))
	}
	return fmt.Sprintf("%s options %s are repeated", e.Directive.Tag(), quoteKeys(e.Keys))
}

// RepeatedNames checks directives that use custom option syntax.
func RepeatedNames(name Name, names []string) error {
	opts := newOptions(len(names))
	var rep repeats
	for _, n := range names {
		rep.add(opts.put(n, ""))
	}
	return rep.err(name)
}

func RepeatedOption(name Name, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return &RepeatedOptionsError{Directive: name, Keys: keys}
}

// Whatever is left after the caller popped every key it knows about.
func (o Options) Unknown(name Name) error {
	if len(o.vals) == 0 {
		return nil
	}
	return UnknownOption(name, o.Keys()...)
}

// What is left for a caller that popped every option it knows: aliases of one
// option given together, and options nobody claimed.
func (o Options) Leftover(name Name) error {
	return errors.Join(o.Conflicts(name), o.Unknown(name))
}

type AliasConflictError struct {
	Directive Name
	Keys      []string
}

func (e *AliasConflictError) Error() string {
	return fmt.Sprintf("%s options %s are the same option", e.Directive.Tag(), quoteKeys(e.Keys))
}

func (o Options) Conflicts(name Name) error {
	if len(o.clash) == 0 {
		return nil
	}
	var errs []error
	for _, group := range slices.Sorted(maps.Keys(o.clash)) {
		errs = append(errs, &AliasConflictError{Directive: name, Keys: o.clash[group]})
	}
	return errors.Join(errs...)
}

// Profile headers use [scope] [name] followed by options. A leading option
// means the name was omitted.
type ProfileHeader struct {
	Scope   Scope
	Name    string
	Options Options
}

func ParseProfileHeader(name Name, rest string) (ProfileHeader, bool, error) {
	fields := fieldsEscaped(rest)
	if len(fields) == 0 {
		return ProfileHeader{}, false, nil
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
	opts, err := parseOptions(name, fields[i:])
	head.Options = opts
	return head, true, err
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
func CutOptions(name Name, input string) (head string, opts Options, err error) {
	lex := &lexer{src: input, escapes: true}
	for {
		tok, ok := lex.next()
		if !ok {
			return strings.TrimSpace(input), newOptions(0), nil
		}
		if isOption(input[tok.start:tok.end]) {
			opts, err = ParseOptions(name, input[tok.start:])
			return strings.TrimSpace(input[:tok.start]), opts, err
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
