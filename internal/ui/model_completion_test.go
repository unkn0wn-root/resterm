package ui

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestBuildCompletionScope(t *testing.T) {
	doc := &restfile.Document{
		Globals:   []restfile.Variable{{Name: "gToken", Secret: true}},
		Variables: []restfile.Variable{{Name: "host"}},
		Requests: []*restfile.Request{
			{Variables: []restfile.Variable{{Name: "reqId"}}},
		},
		Constants: []restfile.Constant{{Name: "apiVersion"}},
		Patches:   []restfile.PatchProfile{{Name: "jsonApi"}},
		SSH:       []restfile.SSHProfile{{Name: "edge"}},
		K8s:       []restfile.K8sProfile{{Name: "cluster"}},
	}
	set := vars.EnvironmentSet{
		vars.SharedEnvKey: {"shared": "1"},
		"dev":             {"host": "dev.local", "apiBase": "https://dev"},
		"prod":            {"host": "prod.local"},
	}

	cat, err := vars.NewCatalog(set)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	sel, err := cat.Select("dev", nil)
	if err != nil {
		t.Fatalf("selection: %v", err)
	}
	scope := buildCompletionScope(doc, cat, sel)

	byName := make(map[string]intellisense.VarRef, len(scope.Variables))
	for _, v := range scope.Variables {
		byName[v.Name] = v
	}

	// A declared variable wins over the environment key of the same name.
	if got := byName["host"]; got.Origin != "file" {
		t.Fatalf("host origin = %q, want file", got.Origin)
	}
	if got := byName["gToken"]; !got.Secret || got.Origin != "global" {
		t.Fatalf("gToken = %+v, want global secret", got)
	}
	if got := byName["apiBase"]; got.Origin != "env" {
		t.Fatalf("apiBase origin = %q, want env", got.Origin)
	}
	for _, name := range []string{"reqId", "apiVersion"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("expected variable %q in scope", name)
		}
	}

	if want := []string{"dev", "prod"}; !reflect.DeepEqual(scope.Environments, want) {
		t.Fatalf(
			"environments = %v, want %v (sorted, no %s)",
			scope.Environments,
			want,
			vars.SharedEnvKey,
		)
	}

	wantProfiles := intellisense.ProfileSet{
		Patch: []string{"jsonApi"},
		SSH:   []string{"edge"},
		K8s:   []string{"cluster"},
	}
	if !reflect.DeepEqual(scope.Profiles, wantProfiles) {
		t.Fatalf("profiles = %+v, want %+v", scope.Profiles, wantProfiles)
	}
}

func TestBuildCompletionScopeNilDocument(t *testing.T) {
	scope := buildCompletionScope(nil, vars.Catalog{}, vars.Selection{})
	if len(scope.Variables) != 0 || len(scope.Environments) != 0 {
		t.Fatalf("expected empty scope for nil document, got %+v", scope)
	}
}

func TestBuildCompletionScopeGroupedEnvironments(t *testing.T) {
	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: vars.EnvironmentSet{
				"dev":  {"api.url": "dev"},
				"prod": {"api.url": "prod"},
			},
		},
		{
			Name: "app",
			Profiles: vars.EnvironmentSet{
				"dev app 1": {"app.url": "one"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	scope := buildCompletionScope(nil, cat, cat.DefaultSelection())
	if len(scope.Environments) != 0 {
		t.Fatalf("flat environments = %#v, want none", scope.Environments)
	}
	want := map[string][]string{
		"api": {"dev", "prod"},
		"app": {"dev app 1"},
	}
	if !reflect.DeepEqual(scope.EnvironmentGroups, want) {
		t.Fatalf("environment groups = %#v, want %#v", scope.EnvironmentGroups, want)
	}
}
