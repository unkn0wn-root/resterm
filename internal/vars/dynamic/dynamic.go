// Package dynamic implements the built-in {{$...}} template helpers.
// A helper is one entry in the table in builtins.go. Parsing, argument
// validation and error wording live here, so adding a helper never touches
// the resolver.
package dynamic

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/duration"
)

// ErrUnknown reports a reference that no helper claims. Callers use it to
// tell a helper apart from an ordinary variable whose name starts with '$'.
var ErrUnknown = errors.New("unknown template helper")

// helper is one built-in. Names match case-insensitively. It stays private
// so the live table cannot be reached from outside the package.
type helper struct {
	name    string
	aliases []string
	summary string
	usage   string // example call, empty when the helper takes no arguments
	offset  bool   // accepts a trailing "+/- duration"
	args    arity
	eval    func(call) (string, error)
}

// Descriptor is a helper as documentation and completions see it. Every copy
// owns its data, so callers cannot reach the registry through one.
type Descriptor struct {
	name    string
	aliases []string
	summary string
	usage   string
}

func (d Descriptor) Name() string {
	return d.name
}

func (d Descriptor) Aliases() []string {
	return slices.Clone(d.aliases)
}

func (d Descriptor) Summary() string {
	return d.summary
}

// Usage is an example call, empty when the helper takes no arguments.
func (d Descriptor) Usage() string {
	return d.usage
}

// arity bounds the argument count. A zero value means "no arguments";
// max below zero means unbounded.
type arity struct {
	min, max int
}

type call struct {
	helper helper
	args   []string
	offset time.Duration
}

func (c call) argInt(i int) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(c.args[i]), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a whole number: %s", c.helper.name, c.args[i], c.helper.usage)
	}
	return n, nil
}

// Resolve renders one helper reference, without the surrounding braces.
// ErrUnknown means no helper matches the name, which callers treat as "not a
// helper". Any other error means the reference itself is wrong.
func Resolve(ref string) (string, error) {
	c, err := parse(ref)
	if err != nil {
		return "", err
	}
	return c.helper.eval(c)
}

// Validate reports whether ref is usable, for checks that run before the
// request or mock response does. It renders and drops the value because
// bounds and ranges are only known to the evaluator, and one shared path
// cannot accept what rendering later rejects.
func Validate(ref string) error {
	_, err := Resolve(ref)
	return err
}

// Helpers describes every built-in, in documentation order.
func Helpers() []Descriptor {
	out := make([]Descriptor, len(builtins))
	for i, h := range builtins {
		out[i] = h.describe()
	}
	return out
}

func (h helper) describe() Descriptor {
	return Descriptor{
		name:    h.name,
		aliases: slices.Clone(h.aliases),
		summary: h.summary,
		usage:   h.usage,
	}
}

func parse(ref string) (call, error) {
	name, args, err := splitCall(ref)
	if err != nil {
		return call{}, err
	}

	var offset time.Duration
	hasOffset := false
	h, ok := lookup(name)
	if !ok {
		// Only a reference that matches nothing is worth splitting: no helper
		// name carries a sign, while a variable name may ("$my-var + 1h").
		if base, dur, split := splitOffset(name); split {
			h, ok = lookup(base)
			offset, hasOffset = dur, true
		}
		if !ok {
			return call{}, fmt.Errorf("%w: %s", ErrUnknown, strings.TrimSpace(ref))
		}
	}

	if hasOffset && !h.offset {
		return call{}, fmt.Errorf("%s does not accept a time offset", h.name)
	}
	if n := len(args); n < h.args.min || (h.args.max >= 0 && n > h.args.max) {
		return call{}, h.arityError(n)
	}
	return call{helper: h, args: args, offset: offset}, nil
}

func (h helper) arityError(n int) error {
	var want string
	switch {
	case h.args.max == 0:
		want = "no arguments"
	case h.args.max < 0:
		want = fmt.Sprintf("at least %s", plural(h.args.min))
	case h.args.min == 0:
		want = fmt.Sprintf("at most %s", plural(h.args.max))
	case h.args.min == h.args.max:
		want = plural(h.args.min)
	default:
		want = fmt.Sprintf("%d to %s", h.args.min, plural(h.args.max))
	}
	if h.usage != "" {
		return fmt.Errorf("%s takes %s, got %d: %s", h.name, want, n, h.usage)
	}
	return fmt.Errorf("%s takes %s, got %d", h.name, want, n)
}

func plural(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", n)
}

// splitCall turns `$randomChoice("a", "b")` into "$randomChoice" and
// ["a", "b"]. A reference without a trailing argument list has no arguments.
func splitCall(ref string) (string, []string, error) {
	ref = strings.TrimSpace(ref)
	open := strings.Index(ref, "(")
	if open < 0 || !strings.HasSuffix(ref, ")") {
		return ref, nil, nil
	}

	name := strings.TrimSpace(ref[:open])
	args, err := splitArgs(ref[open+1 : len(ref)-1])
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", name, err)
	}
	return name, args, nil
}

// splitArgs splits an argument list on commas. Arguments may use either quote
// form, and a backslash inside quotes escapes the next character. Space around
// an argument is dropped, space inside quotes is kept, so `"a b" , c` yields
// "a b" and "c". Trailing space is only known to be trailing once the argument
// ends, so it waits in space until a later rune proves it was interior.
func splitArgs(in string) ([]string, error) {
	var (
		args  []string
		arg   strings.Builder
		space strings.Builder
		quote rune
		esc   bool
	)
	write := func(r rune, quoted bool) {
		if !quoted && unicode.IsSpace(r) {
			if arg.Len() > 0 {
				space.WriteRune(r)
			}
			return
		}
		if space.Len() > 0 {
			arg.WriteString(space.String())
			space.Reset()
		}
		arg.WriteRune(r)
	}
	flush := func() {
		args = append(args, arg.String())
		arg.Reset()
		space.Reset()
	}

	quotedArg := false
	for _, r := range in {
		switch {
		case esc:
			write(r, true)
			esc = false
		case quote != 0 && r == '\\':
			esc = true
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			write(r, true)
		case r == '"' || r == '\'':
			quote, quotedArg = r, true
		case r == ',':
			flush()
			quotedArg = false
		default:
			write(r, false)
		}
	}
	if quote != 0 || esc {
		return nil, errors.New("unterminated quoted argument")
	}
	// An empty list is a call without arguments, not one blank argument.
	if len(args) == 0 && !quotedArg && arg.Len() == 0 {
		return nil, nil
	}
	flush()
	return args, nil
}

// splitOffset splits "$helper +/- duration" into the base name and a signed
// offset. "$timestampISO8601 - 90m" becomes "$timestampISO8601" and -90m.
func splitOffset(name string) (string, time.Duration, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", 0, false
	}
	for i := len(name) - 1; i > 0; i-- {
		ch := name[i]
		if ch != '+' && ch != '-' {
			continue
		}
		base := strings.TrimSpace(name[:i])
		raw := strings.TrimSpace(name[i+1:])
		if base == "" || raw == "" {
			continue
		}
		dur, ok := duration.Parse(raw)
		if !ok {
			continue
		}
		if ch == '-' {
			dur = -dur
		}
		return base, dur, true
	}
	return "", 0, false
}
