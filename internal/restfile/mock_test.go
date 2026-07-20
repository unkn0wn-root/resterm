package restfile

import (
	"encoding/json"
	"slices"
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

func TestMockHeaderRuleJSONRoundTrip(t *testing.T) {
	rules := map[string]MockHeaderRule{
		"exact":   {Op: MockHeaderOpExact, Values: []string{"one", "two"}},
		"prefix":  {Op: MockHeaderOpPrefix, Values: []string{"Bearer "}},
		"present": {Op: MockHeaderOpPresent},
		"absent":  {Op: MockHeaderOpAbsent},
	}
	for name, rule := range rules {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(rule)
			if err != nil {
				t.Fatal(err)
			}
			var got MockHeaderRule
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Op != rule.Op || !slices.Equal(got.Values, rule.Values) {
				t.Fatalf("round trip = %+v, want %+v", got, rule)
			}
		})
	}
	var rule MockHeaderRule
	if err := json.Unmarshal([]byte("null"), &rule); err == nil {
		t.Fatal("null header matcher was accepted")
	}
	if err := json.Unmarshal([]byte("[]"), &rule); err == nil {
		t.Fatal("empty exact matcher was accepted")
	}
	if _, err := json.Marshal(MockHeaderRule{Op: MockHeaderOpExact}); err == nil {
		t.Fatal("empty exact matcher was marshaled")
	}
}

func TestCompileMockPathRejectsRepeatedWildcardNames(t *testing.T) {
	_, _, err := CompileMockPath("/compare/{id}/{id}")
	if err == nil || !strings.Contains(err.Error(), "is repeated") {
		t.Fatalf("CompileMockPath() error = %v", err)
	}
}
