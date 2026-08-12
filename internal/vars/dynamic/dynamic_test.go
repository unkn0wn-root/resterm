package dynamic

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveUnknownHelper(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{"$nope", "$my-custom-var + 1h", "$timestamp + missing", "host"} {
		if _, err := Resolve(ref); !errors.Is(err, ErrUnknown) {
			t.Fatalf("Resolve(%q) error = %v, want ErrUnknown", ref, err)
		}
	}
}

func TestResolveMatchesNamesCaseInsensitively(t *testing.T) {
	t.Parallel()

	for _, ref := range []string{"$UUID", "$Guid", "$TIMESTAMPISO8601", "$timestampms", "$FAKE.Company"} {
		if _, err := Resolve(ref); err != nil {
			t.Fatalf("Resolve(%q): %v", ref, err)
		}
	}
}

func TestResolveTimestampOffsets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want time.Duration
	}{
		{ref: "$timestamp + 2s", want: 2 * time.Second},
		{ref: "$timestampMs - 90m", want: -90 * time.Minute},
		{ref: "$timestampISO8601-1h", want: -time.Hour},
		{ref: "$timestamp + 6d", want: 6 * 24 * time.Hour},
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			t.Parallel()

			start := time.Now().Add(test.want)
			out, err := Resolve(test.ref)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", test.ref, err)
			}
			end := time.Now().Add(test.want)
			got := parseStamp(t, test.ref, out)
			if got.Before(start.Truncate(time.Second)) || got.After(end) {
				t.Fatalf("Resolve(%q) = %q, want between %v and %v", test.ref, out, start, end)
			}
		})
	}
}

func TestValidateRejectsMisuse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want string
	}{
		{ref: "$uuid + 1s", want: "$uuid does not accept a time offset"},
		{ref: "$randomChoice()", want: "$randomChoice takes at least 1 argument, got 0"},
		{ref: `$randomString(8, "hex")`, want: "$randomString takes at most 1 argument, got 2"},
		{ref: "$randomInt(1, 2, 3)", want: "$randomInt takes at most 2 arguments, got 3"},
		{ref: "$uuid(1)", want: "$uuid takes no arguments, got 1"},
		{ref: `$randomChoice("a)`, want: "unterminated quoted argument"},
		{ref: "$randomInt(soon)", want: `"soon" is not a whole number`},
		{ref: "$randomString(0)", want: "length must be between 1 and 4096"},
		{ref: "$randomInt(9, 4)", want: "minimum 9 is above maximum 4"},
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			t.Parallel()

			_, err := Resolve(test.ref)
			if err == nil {
				t.Fatalf("Resolve(%q) succeeded, want error", test.ref)
			}
			if errors.Is(err, ErrUnknown) {
				t.Fatalf("Resolve(%q) reported the helper as unknown: %v", test.ref, err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve(%q) error = %v, want it to contain %q", test.ref, err, test.want)
			}
		})
	}
}

// A reference rejected only at render time reaches the user as a failed
// request instead of a broken file, so the two must agree, bounds and ranges
// included.
func TestValidateAgreesWithResolve(t *testing.T) {
	t.Parallel()

	refs := []string{
		"$uuid",
		"$timestamp + 1h",
		`$randomChoice("a", "b")`,
		"$randomString(4)",
		"$randomInt(1, 6)",
		"$nope",
		"$uuid + 1s",
		"$randomChoice()",
		"$randomString(0)",
		"$randomString(9999)",
		"$randomInt(soon)",
		"$randomInt(9, 4)",
	}
	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			t.Parallel()

			_, resolveErr := Resolve(ref)
			validateErr := Validate(ref)
			if (validateErr == nil) != (resolveErr == nil) {
				t.Fatalf("Validate(%q) = %v, but Resolve = %v", ref, validateErr, resolveErr)
			}
			if errors.Is(validateErr, ErrUnknown) != errors.Is(resolveErr, ErrUnknown) {
				t.Fatalf("Validate(%q) and Resolve disagree on ErrUnknown: %v, %v", ref, validateErr, resolveErr)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "blank", in: "   ", want: nil},
		{name: "bare", in: "a, b ,c", want: []string{"a", "b", "c"}},
		{name: "quoted keeps spaces", in: `" a ", "b"`, want: []string{" a ", "b"}},
		{name: "single quotes", in: `'a', "b"`, want: []string{"a", "b"}},
		{name: "comma inside quotes", in: `"a, b", c`, want: []string{"a, b", "c"}},
		{name: "escaped quote", in: `"say \"hi\""`, want: []string{`say "hi"`}},
		{name: "empty quoted", in: `""`, want: []string{""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := splitArgs(test.in)
			if err != nil {
				t.Fatalf("splitArgs(%q): %v", test.in, err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("splitArgs(%q) = %q, want %q", test.in, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("splitArgs(%q) = %q, want %q", test.in, got, test.want)
				}
			}
		})
	}
}

func TestSplitOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     string
		base   string
		offset time.Duration
		ok     bool
	}{
		{name: "no space", in: "$timestampISO8601-1h", base: "$timestampISO8601", offset: -time.Hour, ok: true},
		{name: "hyphenated base", in: "$my-custom-var + 1h", base: "$my-custom-var", offset: time.Hour, ok: true},
		{name: "no offset", in: "$timestamp", ok: false},
		{name: "unparsable", in: "$timestamp + soon", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			base, offset, ok := splitOffset(test.in)
			if ok != test.ok {
				t.Fatalf("splitOffset(%q) ok = %t, want %t", test.in, ok, test.ok)
			}
			if !ok {
				return
			}
			if base != test.base || offset != test.offset {
				t.Fatalf("splitOffset(%q) = %q, %v, want %q, %v", test.in, base, offset, test.base, test.offset)
			}
		})
	}
}

// Nothing else forces the table to stay consistent with what the parser and
// the completion list expect of it.
func TestRegistryIsWellFormed(t *testing.T) {
	t.Parallel()

	seen := make(map[string]string)
	for _, h := range builtins {
		for _, name := range append([]string{h.name}, h.aliases...) {
			if !strings.HasPrefix(name, "$") {
				t.Fatalf("helper %q must start with '$'", name)
			}
			if strings.ContainsAny(name, "+-() ") {
				t.Fatalf("helper %q must not contain call or offset syntax", name)
			}
			key := strings.ToLower(name)
			if prev, dup := seen[key]; dup {
				t.Fatalf("helper %q collides with %q", name, prev)
			}
			seen[key] = name
		}
		if h.summary == "" {
			t.Fatalf("helper %s needs a summary", h.name)
		}
		if (h.args.max != 0) != (h.usage != "") {
			t.Fatalf("helper %s must document its arguments with usage", h.name)
		}
		if h.eval == nil {
			t.Fatalf("helper %s has no eval", h.name)
		}
	}
}

// Descriptors go to other packages, so editing one must not reach the table.
func TestDescriptorsShareNoStateWithRegistry(t *testing.T) {
	t.Parallel()

	var target Descriptor
	for _, d := range Helpers() {
		if len(d.Aliases()) > 0 {
			target = d
			break
		}
	}
	if target.Name() == "" {
		t.Fatal("no helper with aliases to check")
	}

	alias := target.Aliases()[0]
	target.Aliases()[0] = "$mutated"
	if got := target.Aliases()[0]; got != alias {
		t.Fatalf("descriptor kept the edit: alias = %q, want %q", got, alias)
	}

	for _, d := range Helpers() {
		if d.Name() != target.Name() {
			continue
		}
		if got := d.Aliases()[0]; got != alias {
			t.Fatalf("registry alias = %q, want %q", got, alias)
		}
	}
	if _, err := Resolve(alias); err != nil {
		t.Fatalf("Resolve(%q) after the edit: %v", alias, err)
	}
}

// A new entry cannot ship broken: every helper renders from the form it
// documents.
func TestEveryHelperResolves(t *testing.T) {
	t.Parallel()

	for _, h := range builtins {
		ref := h.name
		if h.args.min > 0 {
			ref = h.usage
		}
		out, err := Resolve(ref)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", ref, err)
		}
		if strings.TrimSpace(out) == "" {
			t.Fatalf("Resolve(%q) returned nothing", ref)
		}
		if h.usage != "" {
			if _, err := Resolve(h.usage); err != nil {
				t.Fatalf("Resolve(%q): %v", h.usage, err)
			}
		}
	}
}

func parseStamp(t *testing.T, ref, value string) time.Time {
	t.Helper()

	if strings.Contains(strings.ToLower(ref), "iso8601") {
		stamp, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatalf("Resolve(%q) = %q, want RFC3339", ref, value)
		}
		return stamp
	}

	unit := time.Second
	if strings.Contains(strings.ToLower(ref), "ms") {
		unit = time.Millisecond
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		t.Fatalf("Resolve(%q) = %q, want a number", ref, value)
	}
	return time.Unix(0, n*int64(unit))
}
