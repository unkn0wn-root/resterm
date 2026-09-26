package restfile

import (
	"maps"
	"testing"
)

func TestOriginFormats(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"auth path and line", (&AuthSpec{SourcePath: "a.http", Line: 3}).Origin(), "a.http:3"},
		{"auth path only", (&AuthSpec{SourcePath: "a.http"}).Origin(), "a.http"},
		{"auth line only", (&AuthSpec{Line: 3}).Origin(), "line 3"},
		{"auth empty", (&AuthSpec{}).Origin(), ""},
		{"auth nil", (*AuthSpec)(nil).Origin(), ""},
		{
			"request path and line",
			(&Request{SourcePath: "b.http", LineRange: LineRange{Start: 10}}).Origin(),
			"b.http:10",
		},
		{"request empty", (&Request{}).Origin(), ""},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestAuthSpecResolveLaysOverridesOnProfile(t *testing.T) {
	prof := AuthProfile{
		Name: "gh",
		Spec: AuthSpec{
			Type:       AuthCommand,
			Params:     map[string]string{"cmd": "gh auth token", "header": "X-Token", "ttl": "5m"},
			SourcePath: "/ws/defs.http",
			Line:       2,
		},
	}
	ref := &AuthSpec{
		Use:        "GH",
		Params:     map[string]string{"header": "Authorization"},
		SourcePath: "/ws/api.http",
		Line:       9,
	}

	got := ref.Resolve(prof)
	want := map[string]string{"cmd": "gh auth token", "header": "Authorization", "ttl": "5m"}
	if !maps.Equal(got.Params, want) {
		t.Fatalf("params = %v, want %v", got.Params, want)
	}
	if got.Kind() != AuthCommand || got.Use != "" || got.Profile != "gh" {
		t.Fatalf("expected a resolved command spec for profile gh, got %+v", got)
	}
	if got.Origin() != "/ws/defs.http:2" {
		t.Fatalf("expected the definition's origin, got %q", got.Origin())
	}
	if prof.Spec.Params["header"] != "X-Token" {
		t.Fatalf("resolve must not change the profile, got %v", prof.Spec.Params)
	}
}
