package httpheader

import (
	"errors"
	"reflect"
	"testing"
)

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
	src := map[string][]string{"X-Test": {"a", "b"}, "Empty": nil}
	got, err := Normalize(src)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want := Values{"x-test": {"a", "b"}, "empty": nil}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
	src["X-Test"][0] = "changed"
	if got["x-test"][0] != "a" {
		t.Fatal("Normalize retained the caller's value slice")
	}
}
