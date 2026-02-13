package collection

import (
	"path/filepath"
	"testing"
)

func TestNormRelPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "requests.http", want: "requests.http", ok: true},
		{in: "./dir/../requests.http", want: "requests.http", ok: true},
		{in: "rts\\helpers.rts", want: "rts/helpers.rts", ok: true},
		{in: "", ok: false},
		{in: ".", ok: false},
		{in: "..", ok: false},
		{in: "../x.http", ok: false},
		{in: "a/../../x.http", ok: false},
		{in: "/etc/passwd", ok: false},
		{in: "\\\\server\\share\\x", ok: false},
		{in: "C:/tmp/x.http", ok: false},
		{in: "C:\\tmp\\x.http", ok: false},
	}

	for _, tc := range tests {
		got, err := NormRelPath(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("NormRelPath(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormRelPath(%q)=%q want %q", tc.in, got, tc.want)
			}
			continue
		}
		if err == nil {
			t.Fatalf("NormRelPath(%q) expected error", tc.in)
		}
	}
}

func TestSafeJoinRejectsTraversal(t *testing.T) {
	base := t.TempDir()

	got, err := SafeJoin(base, "nested/req.http")
	if err != nil {
		t.Fatalf("SafeJoin unexpected error: %v", err)
	}
	if !withinBase(base, got) {
		t.Fatalf("joined path escaped base: %q", got)
	}

	bad := []string{
		"../x.http",
		"..\\x.http",
		"/tmp/x.http",
		"C:/tmp/x.http",
	}
	for _, rel := range bad {
		if _, err := SafeJoin(base, rel); err == nil {
			t.Fatalf("SafeJoin(%q) expected error", rel)
		}
	}
}

func TestWithinBase(t *testing.T) {
	base := t.TempDir()
	okPath := filepath.Join(base, "a", "b.http")
	badPath := filepath.Join(base, "..", "escape.http")

	if !withinBase(base, okPath) {
		t.Fatalf("expected path to be inside base")
	}
	if withinBase(base, badPath) {
		t.Fatalf("expected path to be outside base")
	}
}
