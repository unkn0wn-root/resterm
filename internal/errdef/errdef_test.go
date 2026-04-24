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

func TestHasCodeFindsWrappedAndJoinedCodes(t *testing.T) {
	timeout := New(CodeTimeout, "timed out")
	auth := Wrap(CodeAuth, timeout, "resolve auth")
	joined := Join(CodeProtocol, errors.New("plain"), auth)

	if !HasCode(joined, CodeTimeout) {
		t.Fatalf("expected joined error to contain %q", CodeTimeout)
	}
	if !HasCode(joined, CodeAuth) {
		t.Fatalf("expected joined error to contain %q", CodeAuth)
	}
	if !HasCode(joined, CodeProtocol) {
		t.Fatalf("expected joined error to contain %q", CodeProtocol)
	}
	if HasCode(joined, CodeTLS) {
		t.Fatalf("did not expect joined error to contain %q", CodeTLS)
	}
}

func TestCodesReturnsWrappedAndJoinedCodes(t *testing.T) {
	timeout := New(CodeTimeout, "timed out")
	auth := Wrap(CodeAuth, timeout, "resolve auth")
	joined := Join(CodeProtocol, errors.New("plain"), auth, New(CodeAuth, "again"))

	got := Codes(joined)
	want := []Code{CodeProtocol, CodeAuth, CodeTimeout}
	if len(got) != len(want) {
		t.Fatalf("Codes(error) = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Codes(error) = %#v, want %#v", got, want)
		}
	}
}
