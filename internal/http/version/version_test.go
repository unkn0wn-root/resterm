package version

import (
	"errors"
	"testing"
)

func TestParseToken(t *testing.T) {
	cases := map[string]HTTP{
		"HTTP/1.1": V11,
		"http/2":   V2,
		"http/2.0": V2,
	}
	for raw, want := range cases {
		got, ok := ParseToken(raw)
		if !ok || got != want {
			t.Fatalf("ParseToken(%q) = %v, %v", raw, got, ok)
		}
	}

	if _, ok := ParseToken("1.1"); ok {
		t.Fatalf("expected bare version to be rejected")
	}
}

func TestParseValue(t *testing.T) {
	cases := map[string]HTTP{
		"1.1":      V11,
		"2":        V2,
		"2.0":      V2,
		"HTTP/1.1": V11,
		"HTTP/2":   V2,
	}
	for raw, want := range cases {
		got, ok := ParseValue(raw)
		if !ok || got != want {
			t.Fatalf("ParseValue(%q) = %v, %v", raw, got, ok)
		}
	}

	for _, raw := range []string{"HTTP/3", "1.0", "HTTP/1.0"} {
		if _, ok := ParseValue(raw); ok {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestSplitToken(t *testing.T) {
	for token, want := range map[string]HTTP{
		"HTTP/1.1": V11,
		"http/2":   V2,
		"HTTP/2.0": V2,
	} {
		out, got, err := SplitToken([]string{"http://example.com", token})
		if err != nil {
			t.Fatalf("SplitToken(%q): %v", token, err)
		}
		if got != want || len(out) != 1 || out[0] != "http://example.com" {
			t.Fatalf("SplitToken(%q) = %#v, %v", token, out, got)
		}
	}
}

func TestSplitTokenRejectsUnsupportedVersion(t *testing.T) {
	for _, token := range []string{"HTTP/3", "HTTP/1.0", "HTTP/11"} {
		out, v, err := SplitToken([]string{"http://example.com/bad", token})
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("SplitToken(%q) error = %v, want *UnsupportedError", token, err)
		}
		if unsupported.Token != token {
			t.Fatalf("token = %q, want %q", unsupported.Token, token)
		}
		if v != Unknown || len(out) != 2 {
			t.Fatalf("expected the fields to be left alone, got %#v %v", out, v)
		}
	}
}

func TestSplitTokenKeepsNonVersionTail(t *testing.T) {
	tails := []string{
		"b", "http/foo", "HTTP/", "HTTP/2/x", "http/2?a=b", "http/1.1.1",
	}
	for _, tail := range tails {
		out, v, err := SplitToken([]string{"http://example.com/a", tail})
		if err != nil {
			t.Fatalf("SplitToken(%q): %v", tail, err)
		}
		if v != Unknown || len(out) != 2 {
			t.Fatalf("SplitToken(%q) split the tail off: %#v %v", tail, out, v)
		}
	}
}

func TestSetIfMissing(t *testing.T) {
	m := map[string]string{"HTTP-Version": "2"}
	out := SetIfMissing(m, V11)
	if out["HTTP-Version"] != "2" {
		t.Fatalf("expected existing key to remain")
	}
	if out[Key] != "" {
		t.Fatalf("expected no new key to be set")
	}
}
