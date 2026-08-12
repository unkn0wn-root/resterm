package header

import (
	"errors"
	"reflect"
	"testing"
)

func TestValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "letters digits and punctuation", input: "X-Test_1!", want: true},
		{name: "empty", input: "", want: false},
		{name: "space", input: "X Test", want: false},
		{name: "colon", input: "X-Test:", want: false},
		{name: "non-ascii", input: "X-Tést", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Valid(test.input); got != test.want {
				t.Fatalf("Valid(%q) = %t, want %t", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeRejectsInvalidAndEquivalentNames(t *testing.T) {
	_, err := Normalize(map[string][]string{"X Bad": {"a"}})
	var nameErr *NameError
	if !errors.As(err, &nameErr) {
		t.Fatalf("expected NameError, got %v", err)
	}

	_, err = Normalize(map[string][]string{"X-Test": {"a"}, "x-test": {"b"}})
	var collision *CollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("expected CollisionError, got %v", err)
	}
}

func TestParseReturnsCaseInsensitiveIdentityWithoutTrimming(t *testing.T) {
	n, err := Parse("X-Test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if n.Key() != "x-test" {
		t.Fatalf("Name.Key() = %q, want x-test", n.Key())
	}
	if _, err := Parse(" X-Test "); err == nil {
		t.Fatal("Parse accepted surrounding whitespace")
	}
}

func TestNormalizeKeepsEveryValueAsAList(t *testing.T) {
	src := map[string][]string{
		"X-Test":    {"a", "b"},
		"Empty":     nil,
		"No-Values": {},
	}
	got, err := Normalize(src)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want := Values{"x-test": {"a", "b"}, "empty": nil, "no-values": {}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
	src["X-Test"][0] = "changed"
	if got["x-test"][0] != "a" {
		t.Fatal("Normalize retained the caller's value slice")
	}
}
