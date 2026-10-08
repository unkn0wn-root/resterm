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

// All writes go through put. Lookups assume keys are lowercase.
func (o Options) put(key, val string) (repeated string) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	_, seen := o.vals[key]
	// Values are already decoded. Keep any quotes that belong to the value itself.
	o.vals[key] = strings.TrimSpace(val)
	if !seen {
		return ""
	}
	return key
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

func (o Options) Get(key string) string {
	return o.vals[key]
}

// First returns the first non-empty value in key order.
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

// PopAny removes all aliases and returns the first non-empty value.
// Removing unused aliases prevents false unknown-option errors.
func (o Options) PopAny(keys ...string) (string, bool) {
	_, val, ok := o.PopKey(keys...)
	return val, ok
}

// PopKey removes all aliases and returns the first with a non-empty value.
// The key identifies the alias to use in diagnostics.
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

// PopBool removes the aliases and reads the first as a boolean. An empty value
// means true; an invalid value is returned in bad for the caller to report.
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

func (o Options) Unknown(name Name) error {
	return UnknownOption(name, o.Keys()...)
}

// Leftover reports alias conflicts and unconsumed options.
func (o Options) Leftover(name Name) error {
	return errors.Join(o.Conflicts(name), o.Unknown(name))
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

// Empty boolean values still enable the switch, so every supplied alias
// counts when checking for conflicts.
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

// ParseOptions reads key=value options and treats bare keys as true.
// Keys are lowercased; duplicate keys return an error.
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

// OptionFields collects key=value options from fields returned by ScanFields.
// Bare keys are ignored.
func OptionFields(name Name, fields []Field) (Options, error) {
	return collectOptions(name, fields, false)
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

// ProfileHeader holds [scope] [name] followed by options.
// A leading option means the name was omitted.
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
	if i < len(fields) && fields[i].Positional() {
		head.Name = strings.TrimSpace(fields[i].Value)
		i++
	}
	opts, err := parseOptions(name, fields[i:])
	head.Options = opts
	return head, true, err
}

// ParseNameValue accepts a name and value separated by whitespace, : or =.
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
		// No whitespace or separator: the name contains an invalid character.
		return "", ""
	}
	return tr[:end], strings.TrimSpace(val)
}

func parseOptions(name Name, fields []Field) (Options, error) {
	return collectOptions(name, fields, true)
}

func collectOptions(name Name, fields []Field, bareIsTrue bool) (Options, error) {
	opts := newOptions(len(fields))
	var (
		rep    repeats
		spaced []string
		bad    []error
	)
	for i := 0; i < len(fields); i++ {
		// The key alone would read as true, so no field of a spaced option is stored.
		if key, n := SpacedOption(fields, i); n > 0 {
			spaced = append(spaced, key)
			i += n - 1
			continue
		}
		f := fields[i]
		key, val, _ := strings.Cut(f.Value, f.Op.String())
		switch {
		case noKey(f.Value): // =x, or = v with no key before it
			spaced = append(spaced, f.Value)
			if isOp(f.Value) && valueNext(fields, i) {
				i++
			}
		case f.Op == OpEq:
			rep.add(opts.put(key, val))
		case f.Op != OpNone:
			bad = append(bad, &OpOptionError{Directive: name, Key: key, Value: val, Op: f.Op})
		case bareIsTrue:
			rep.add(opts.put(f.Value, "true"))
		}
	}
	err := rep.err(name)
	if len(spaced) > 0 {
		err = errors.Join(err, &SpacedOptionsError{Directive: name, Keys: spaced})
	}
	return opts, errors.Join(append([]error{err}, bad...)...)
}

// SpacedOption returns the key and number of fields in an option split by
// spaces around its operator, or ("", 0) if fields[i] is not one.
func SpacedOption(fields []Field, i int) (string, int) {
	f := fields[i]
	if SpacedKey(fields, i) {
		if isOp(fields[i+1].Value) && valueNext(fields, i+1) {
			return f.Value, 3
		}
		return f.Value, 2
	}
	// k= v needs a value; k="" is already complete. Check the source offsets
	// because both decode to k=.
	if f.Op != OpNone && f.ValueStart() == f.End && valueNext(fields, i) {
		return strings.TrimSuffix(f.Value, f.Op.String()), 2
	}
	return "", 0
}

// SpacedKey reports whether spaces split a key from its operator (k = v or k =v).
func SpacedKey(fields []Field, i int) bool {
	key := fields[i].Value
	return !slices.ContainsFunc(ops, func(op Op) bool { return strings.Contains(key, op.String()) }) &&
		strings.TrimSpace(key) != "" && i+1 < len(fields) && noKey(fields[i+1].Value)
}

func isOp(field string) bool {
	return slices.ContainsFunc(ops, func(op Op) bool { return field == op.String() })
}

func noKey(field string) bool {
	field = strings.TrimSpace(field)
	return slices.ContainsFunc(ops, func(op Op) bool { return strings.HasPrefix(field, op.String()) })
}

// A quoted "a=b" can be a value; an unquoted a=b starts another option.
func valueNext(fields []Field, i int) bool {
	return i+1 < len(fields) && fields[i+1].Op == OpNone
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

// OpOptionError reports an option written with an operator other than =.
type OpOptionError struct {
	Directive  Name
	Key, Value string
	Op         Op
}

func (e *OpOptionError) Error() string {
	return fmt.Sprintf(
		"%s option %q takes = instead of %s. Write it as %s=%s",
		e.Directive.Tag(),
		e.Key,
		e.Op,
		e.Key,
		e.Value,
	)
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

func RepeatedOption(name Name, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return &RepeatedOptionsError{Directive: name, Keys: keys}
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
	var op *OpOptionError
	switch {
	case errors.As(err, &unknown):
		return unknown.Keys
	case errors.As(err, &spaced):
		return spaced.Keys
	case errors.As(err, &op):
		return []string{op.Key}
	case errors.As(err, &repeated):
		return repeated.Keys
	case errors.As(err, &conflict):
		return conflict.Keys
	default:
		return nil
	}
}

func quoteKeys(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = strconv.Quote(key)
	}
	return strings.Join(quoted, ", ")
}
