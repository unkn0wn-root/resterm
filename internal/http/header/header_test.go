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

func TestValueAndPresentIgnoreKeyCaseAndBlanks(t *testing.T) {
	src := map[string][]string{
		"authorization": {"Bearer from-user"},
		"X-Blank":       {"", "  "},
		"X-Second":      {" ", "kept"},
	}
	if got := Value(src, "Authorization"); got != "Bearer from-user" {
		t.Fatalf("Value(Authorization) = %q, want the lowercase entry", got)
	}
	if got := Value(src, "x-second"); got != "kept" {
		t.Fatalf("Value(x-second) = %q, want kept", got)
	}
	if Present(src, "X-Blank") {
		t.Fatal("a blank value counts as present")
	}
	if Present(nil, "Authorization") {
		t.Fatal("a nil block reports a header")
	}
	if Present(src, "X-Missing") {
		t.Fatal("an absent header reports present")
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

func TestSensitive(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "Authorization", want: true},
		{name: "authorization", want: true},
		{name: "  X-API-Key  ", want: true},
		{name: "x-auth-token", want: true},
		{name: "Proxy-Authorization", want: true},
		{name: "Cookie", want: true},
		{name: "Cookie2", want: true},
		{name: "X-Goog-Api-Key", want: true},
		{name: "Content-Type", want: false},
		{name: "X-Request-Id", want: false},
		{name: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sensitive(tt.name); got != tt.want {
				t.Fatalf("Sensitive(%q) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}

func TestSetMatchesNamesByIdentity(t *testing.T) {
	s := NewSet("X-Registry-Token", " x-tenant ", "", "   ")

	for _, name := range []string{"x-registry-token", "X-REGISTRY-TOKEN", " X-Tenant "} {
		if !s.Has(name) {
			t.Errorf("Has(%q) = false, want the stored identity to match", name)
		}
	}
	if s.Has("x-other") {
		t.Error("Has(x-other) = true, want an absent name to miss")
	}
	if len(s) != 2 {
		t.Fatalf("len(Set) = %d, want the blank names dropped", len(s))
	}
	if (Set)(nil).Has("authorization") {
		t.Error("a nil Set reports a name")
	}
}

func TestIsCookie(t *testing.T) {
	for _, name := range []string{"Cookie", "COOKIE2", " cookie "} {
		if !IsCookie(name) {
			t.Errorf("IsCookie(%q) = false", name)
		}
	}
	for _, name := range []string{"Authorization", "Set-Cookie", ""} {
		if IsCookie(name) {
			t.Errorf("IsCookie(%q) = true", name)
		}
	}
}
