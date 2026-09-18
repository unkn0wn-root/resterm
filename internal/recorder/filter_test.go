package recorder

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRuleNormalisesMethod(t *testing.T) {
	for raw, method := range map[string]string{"get /health": "GET", "/health": "", " Post  /users/{id} ": "POST"} {
		r, err := parseRule(raw)
		if err != nil || r.method != method {
			t.Fatalf("rule %q: %+v, %v", raw, r, err)
		}
	}
}

func TestFilterCaptures(t *testing.T) {
	f, err := newFilter(Config{
		Skip: []string{"GET /health", "/assets/{path...}"},
		Only: []string{"/api/{rest...}", "GET /health", "POST /assets/{path...}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]bool{
		"GET /health":           false,
		"POST /health":          false,
		"GET /assets/a/b.css":   false,
		"POST /assets/a.css":    false,
		"GET /api/users":        true,
		"GET /api/users?x=1":    true,
		"GET /api":              false,
		"DELETE /api/users/42":  true,
		"GET /api/users/../etc": false,
	} {
		method, path, _ := strings.Cut(target, " ")
		if got := f.captures(httptest.NewRequest(method, path, nil)); got != want {
			t.Fatalf("%s: captured %v, want %v", target, got, want)
		}
	}
}
