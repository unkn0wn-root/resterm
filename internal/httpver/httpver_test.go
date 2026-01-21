package httpver

import "testing"

func TestParseToken(t *testing.T) {
	cases := map[string]Version{
		"HTTP/1.0": V10,
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
	cases := map[string]Version{
		"1.0":      V10,
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

	if _, ok := ParseValue("HTTP/3"); ok {
		t.Fatalf("expected unknown version to be rejected")
	}
}

func TestSplitToken(t *testing.T) {
	fields := []string{"http://example.com", "HTTP/1.1"}
	out, v := SplitToken(fields)
	if v != V11 {
		t.Fatalf("expected V11, got %v", v)
	}
	if len(out) != 1 || out[0] != "http://example.com" {
		t.Fatalf("unexpected fields: %#v", out)
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
