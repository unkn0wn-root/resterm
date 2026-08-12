package delay

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
)

// Spec is one parsed delay expression; the zero value never delays. What base
// and spread hold depends on the kind: the constant delay, the bounds of
// random, or a center and the deviation around it. A deviation written as a
// percentage stays one so the text round-trips.
type Spec struct {
	kind   kind
	base   time.Duration
	spread time.Duration
	pct    float64
}

func Fixed(d time.Duration) Spec {
	return Spec{base: d}
}

// Parse reads a duration such as "150ms" or a call such as "random(100ms,500ms)".
func Parse(raw string) (Spec, error) {
	raw = strings.TrimSpace(raw)
	name, args, isCall, err := splitCall(raw)
	if err != nil {
		return Spec{}, err
	}
	if !isCall {
		d, ok := duration.Parse(raw)
		switch {
		case !ok:
			return Spec{}, fmt.Errorf("must be a duration such as 150ms or a distribution: %s", formsHint)
		case d < 0:
			return Spec{}, errors.New("cannot be negative")
		}
		return Fixed(d), nil
	}

	k, f, ok := lookup(name)
	if !ok {
		return Spec{}, fmt.Errorf("%q is not a delay distribution: use %s", name, formsHint)
	}
	if len(args) != len(f.params) {
		return Spec{}, fmt.Errorf(
			"%s takes %d arguments, got %d: %s",
			f.name, len(f.params), len(args), f.usage(),
		)
	}

	base, err := f.duration(0, args[0])
	if err != nil {
		return Spec{}, err
	}
	s := Spec{kind: k, base: base}
	switch pct, relative, err := f.percent(1, args[1]); {
	case err != nil:
		return Spec{}, err
	case relative:
		s.pct = pct
	default:
		if s.spread, err = f.duration(1, args[1]); err != nil {
			return Spec{}, err
		}
	}
	if f.check != nil {
		if err := f.check(s); err != nil {
			return Spec{}, err
		}
	}
	return s, nil
}

// Sample returns the delay for one request. It is never negative and is safe
// for concurrent use.
func (s Spec) Sample() time.Duration {
	return s.draw(globalSource{})
}

func (s Spec) draw(r source) time.Duration {
	return forms[s.kind].sample(s, r)
}

func (s Spec) IsZero() bool {
	return s.kind == fixed && s.base <= 0
}

// String is the canonical expression, empty when there is no delay.
func (s Spec) String() string {
	f := forms[s.kind]
	if f.name == "" {
		if s.IsZero() {
			return ""
		}
		return s.base.String()
	}
	return call(f.name, s.base.String()+","+s.argText())
}

func (s Spec) argText() string {
	if s.pct > 0 {
		return strconv.FormatFloat(s.pct, 'f', -1, 64) + "%"
	}
	return s.spread.String()
}

func (s Spec) deviation() time.Duration {
	if s.pct > 0 {
		return clamp(float64(s.base) * s.pct / 100)
	}
	return s.spread
}

// The mock reload digest hashes parsed specs as JSON, so a spec has to encode
// as the text it was written with.
func (s Spec) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *Spec) UnmarshalText(text []byte) error {
	if len(bytes.TrimSpace(text)) == 0 {
		*s = Spec{}
		return nil
	}
	spec, err := Parse(string(text))
	if err != nil {
		return err
	}
	*s = spec
	return nil
}

// Descriptor is one distribution as documentation and completions see it.
type Descriptor struct {
	name    string
	summary string
	args    string
}

func (d Descriptor) Name() string {
	return d.name
}

func (d Descriptor) Summary() string {
	return d.summary
}

// Usage is the example call, e.g. random(100ms,500ms).
func (d Descriptor) Usage() string {
	return call(d.name, d.args)
}

// Args is the example argument list of Usage without the parentheses, so a
// caller offering the usage as a template knows what part of it to fill in.
func (d Descriptor) Args() string {
	return d.args
}

// Distributions describes the call forms, in documentation order.
func Distributions() []Descriptor {
	out := make([]Descriptor, 0, len(forms)-1)
	for _, f := range forms {
		if f.name != "" {
			out = append(out, Descriptor{name: f.name, summary: f.summary, args: f.args})
		}
	}
	return out
}

// A value without a trailing argument list is a plain duration, not a call.
func splitCall(raw string) (name string, args []string, isCall bool, err error) {
	head, tail, ok := strings.Cut(raw, "(")
	if !ok {
		return "", nil, false, nil
	}
	name = strings.TrimSpace(head)
	rest, closed := strings.CutSuffix(tail, ")")
	if !closed {
		return "", nil, false, fmt.Errorf("%s is missing a closing \")\"", name)
	}
	if strings.TrimSpace(rest) == "" {
		return name, nil, true, nil
	}
	args = strings.Split(rest, ",")
	for i, arg := range args {
		args[i] = strings.TrimSpace(arg)
	}
	return name, args, true, nil
}

var formsHint = describeForms()

func describeForms() string {
	usages := make([]string, 0, len(forms))
	for _, f := range forms {
		if usage := f.usage(); usage != "" {
			usages = append(usages, usage)
		}
	}
	if last := len(usages) - 1; last > 0 {
		usages[last] = "or " + usages[last]
	}
	return strings.Join(usages, ", ")
}
