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
	return parseOptions(name, slices.Collect(scanFields(input, true)))
}

// OptionsOpen returns the missing delimiter in the last option value, or zero.
func OptionsOpen(input string) rune {
	return (&lexer{src: input, escapes: true}).open()
}

// FieldsOpen reports an unclosed value using Fields' quote rules.
func FieldsOpen(input string) rune {
	return (&lexer{src: input}).open()
}

// OptionFields is for callers that already separated the input with
// ScanFields. It only keeps key=value pairs, unlike ParseOptions where a bare
// key means true.
func OptionFields(name Name, fields []Field) (Options, error) {
	return collectOptions(name, fields, false)
}

func parseOptions(name Name, fields []Field) (Options, error) {
	return collectOptions(name, fields, true)
}

func collectOptions(name Name, fields []Field, bareIsTrue bool) (Options, error) {
	opts := newOptions(len(fields))
	var (
		rep    repeats
		spaced []string
	)
	for i := 0; i < len(fields); i++ {
		// The key alone would read as true, so no field of a spaced option is stored.
		if key, n := SpacedOption(fields, i, "="); n > 0 {
			spaced = append(spaced, key)
			i += n - 1
			continue
		}
		f := fields[i]
		key, val, ok := strings.Cut(f.Value, "=")
		switch {
		case noKey(f.Value, "="): // =x, or = v with no key before it
			spaced = append(spaced, f.Value)
			if f.Value == "=" && valueNext(fields, i, "=") {
				i++
			}
		case ok:
			rep.add(opts.put(key, val))
		case bareIsTrue:
			rep.add(opts.put(key, "true"))
		}
	}
	err := rep.err(name)
	if len(spaced) > 0 {
		err = errors.Join(err, &SpacedOptionsError{Directive: name, Keys: spaced})
	}
	return opts, err
}

// SpacedOption returns the key and field count of an option at i that spaces split around one of ops.
func SpacedOption(fields []Field, i int, ops ...string) (string, int) {
	f := fields[i]
	if SpacedKey(fields, i, ops...) { // k = v, k =v
		// A lone operator takes the next field as its value.
		if slices.Contains(ops, fields[i+1].Value) && valueNext(fields, i+1, ops...) {
			return f.Value, 3
		}
		return f.Value, 2
	}
	// k= v. Only the source tells k= from k="", so this reads the span.
	for _, op := range ops {
		if at := f.at(op); at >= 0 && at+len(op) == f.End && valueNext(fields, i, ops...) {
			return strings.TrimSuffix(f.Value, op), 2
		}
	}
	return "", 0
}

func noKey(field string, ops ...string) bool {
	field = strings.TrimSpace(field)
	return slices.ContainsFunc(ops, func(op string) bool { return strings.HasPrefix(field, op) })
}

// SpacedKey reports whether field i is a key whose operator was split off by a
// space, as in k = v or k =v.
func SpacedKey(fields []Field, i int, ops ...string) bool {
	key := fields[i].Value
	return !strings.Contains(key, "=") && strings.TrimSpace(key) != "" &&
		i+1 < len(fields) && noKey(fields[i+1].Value, ops...)
}

// The field after i can be a value only if it is not an option of its own. Only
// the source tells a=b from "a=b", so this reads the span.
func valueNext(fields []Field, i int, ops ...string) bool {
	return i+1 < len(fields) && !slices.ContainsFunc(ops, func(op string) bool { return fields[i+1].at(op) >= 0 })
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

// SpacedOptionsError reports options written with spaces around =. Keys holds
// each key as written, or the whole field when it has no key, such as =x.
type SpacedOptionsError struct {
	Directive Name
	Keys      []string
}

func (e *SpacedOptionsError) Error() string {
	if len(e.Keys) == 1 {
		return fmt.Sprintf(
			"%s option %s has spaces around =. Write it as key=value",
			e.Directive.Tag(),
			quoteKeys(e.Keys),
		)
	}
	return fmt.Sprintf(
		"%s options %s have spaces around =. Write them as key=value",
		e.Directive.Tag(),
		quoteKeys(e.Keys),
	)
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

// OptionKeys lists the option names an error reports, across joined errors.
func OptionKeys(err error) []string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var keys []string
		for _, child := range joined.Unwrap() {
			for _, key := range OptionKeys(child) {
				if !slices.Contains(keys, key) {
					keys = append(keys, key)
				}
			}
		}
		return keys
	}
	var unknown *UnknownOptionsError
	var repeated *RepeatedOptionsError
	var conflict *AliasConflictError
	var spaced *SpacedOptionsError
	switch {
	case errors.As(err, &unknown):
		return unknown.Keys
	case errors.As(err, &spaced):
		return spaced.Keys
	case errors.As(err, &repeated):
		return repeated.Keys
	case errors.As(err, &conflict):
		return conflict.Keys
	default:
		return nil
	}
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
	fields := slices.Collect(scanFields(rest, true))
	if len(fields) == 0 {
		return ProfileHeader{}, false, nil
	}

	i := 0
	head := ProfileHeader{Scope: ScopeRequest}
	if scope, ok := ParseScope(fields[i].Value); ok {
		head.Scope = scope
		i++
	}
	if i < len(fields) && !strings.Contains(fields[i].Value, "=") {
		head.Name = strings.TrimSpace(fields[i].Value)
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

// FieldSpan locates one field of an option list in its source text. Offsets are
// bytes. Eq is the equals sign that makes the field an option, -1 when the
// field is positional. Le is the <= after a name, as in dns<=50ms.
type FieldSpan struct {
	Start, End, Eq, Le int
}

func (s FieldSpan) at(op string) int {
	switch op {
	case "=":
		return s.Eq
	case "<=":
		return s.Le
	}
	return -1
}

// Field pairs a decoded option field with its byte offsets in the source.
type Field struct {
	FieldSpan
	Value string
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
			span := FieldSpan{Start: tok.start, End: tok.end, Eq: -1, Le: -1}
			if eq := optionEq(raw); eq >= 0 {
				span.Eq = tok.start + eq
			} else if i := strings.IndexFunc(raw, func(r rune) bool { return !IsKeyRune(r) }); i > 0 &&
				strings.HasPrefix(raw[i:], "<=") {
				span.Le = tok.start + i
			}
			if !yield(Field{FieldSpan: span, Value: tok.val}) {
				return
			}
		}
	}
}

// FieldSpans reports where each field sits, scanning the way ParseOptions does,
// so a span never splits a quoted or bracketed value.
func FieldSpans(input string) []FieldSpan {
	var spans []FieldSpan
	for field := range scanFields(input, true) {
		spans = append(spans, field.FieldSpan)
	}
	return spans
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
