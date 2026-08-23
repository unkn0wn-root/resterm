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
	fields := []string{"http://example.com", "HTTP/1.1"}
	out, v, err := SplitToken(fields)
	if err != nil {
		t.Fatalf("SplitToken: %v", err)
	}
	if v != V11 {
		t.Fatalf("expected V11, got %v", v)
	}
	if len(out) != 1 || out[0] != "http://example.com" {
		t.Fatalf("unexpected fields: %#v", out)
	}
}

func TestSplitTokenRejectsUnsupportedVersion(t *testing.T) {
	for _, token := range []string{"HTTP/9.9", "HTTP/3", "http/0.9", "HTTP/1.0"} {
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
	fields := []string{"http://example.com/a", "b"}
	out, v, err := SplitToken(fields)
	if err != nil {
		t.Fatalf("SplitToken: %v", err)
	}
	if v != Unknown || len(out) != 2 {
		t.Fatalf("unexpected split: %#v %v", out, v)
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
