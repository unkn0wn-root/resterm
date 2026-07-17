package restfile

import (
	"strings"
	"testing"
)

func TestCompileMockPathMapsSourceWildcards(t *testing.T) {
	pattern, params, err := CompileMockPath("/users/{id}/files/{path...}")
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "/users/{p1}/files/{p3...}" {
		t.Fatalf("pattern = %q", pattern)
	}
	if params["id"] != "p1" || params["path"] != "p3" || len(params) != 2 {
		t.Fatalf("params = %#v", params)
	}

	other, _, err := CompileMockPath("/users/{userID}/files/{rest...}")
	if err != nil {
		t.Fatal(err)
	}
	if other != pattern {
		t.Fatalf("equivalent pattern = %q, want %q", other, pattern)
	}
}

func TestCompileMockPathRejectsRepeatedWildcardNames(t *testing.T) {
	_, _, err := CompileMockPath("/compare/{id}/{id}")
	if err == nil || !strings.Contains(err.Error(), "is repeated") {
		t.Fatalf("CompileMockPath() error = %v", err)
	}
}
