package errdef

import (
	"errors"
	"testing"
)

func TestJoinReturnsNilWhenEmpty(t *testing.T) {
	if err := Join(CodeConfig, nil, nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestJoinWrapsJoinedErrorsWithCode(t *testing.T) {
	a := errors.New("first")
	b := errors.New("second")

	err := Join(CodeConfig, a, nil, b)
	if err == nil {
		t.Fatal("expected joined error")
	}
	if got := CodeOf(err); got != CodeConfig {
		t.Fatalf("expected code %q, got %q", CodeConfig, got)
	}
	if !errors.Is(err, a) {
		t.Fatalf("expected joined error to match first child")
	}
	if !errors.Is(err, b) {
		t.Fatalf("expected joined error to match second child")
	}
}
