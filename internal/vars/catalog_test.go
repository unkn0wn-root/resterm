package vars

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadGroupedEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resterm.env.json")
	data := `{
		"$shared": {"region": "eu"},
		"$groups": {
			"app": {
				"$default": "dev app 1",
				"dev app 1": {"app.url": "https://app-1"},
				"dev app 2": {"app.url": "https://app-2"}
			},
			"api": {
				"dev": {"api.url": "https://api"}
			}
		}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cat, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	if !cat.Grouped() {
		t.Fatal("expected grouped catalog")
	}

	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve default: %v", err)
	}
	if got, want := env.Label(), "api=dev, app=dev app 1"; got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
	if got := env.Values()["region"]; got != "eu" {
		t.Fatalf("shared region = %q, want eu", got)
	}
	// A catalog read from a file folds that file into its scope, which the g2
	// prefix marks. Catalogs built in process keep the file-blind g1 scopes.
	if !strings.HasPrefix(env.Scope(), "g2:") {
		t.Fatalf("scope = %q, want g2 prefix", env.Scope())
	}

	sel := env.Selection().WithGroup("app", "dev app 2")
	next, err := cat.Resolve(sel)
	if err != nil {
		t.Fatalf("resolve changed selection: %v", err)
	}
	if got, want := next.Label(), "api=dev, app=dev app 2"; got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
	if next.Scope() == env.Scope() {
		t.Fatal("changing one profile must change runtime scope")
	}
}

func TestGroupedCatalogValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "mixed flat and grouped",
			data: `{"dev": {}, "$groups": {"api": {"dev": {}}}}`,
			want: "cannot be mixed",
		},
		{
			name: "duplicate schema key ignoring case",
			data: `{"$groups": {"api": {"dev": {}}}, " $GROUPS ": {}}`,
			want: "duplicate schema names",
		},
		{
			name: "duplicate group ignoring case",
			data: `{"$groups": {"api": {"dev": {}}, "API": {"dev": {}}}}`,
			want: "duplicate group names",
		},
		{
			name: "duplicate profile ignoring case",
			data: `{"$groups": {"api": {"dev": {}, "DEV": {}}}}`,
			want: "duplicate profile",
		},
		{
			name: "blank group",
			data: `{"$groups": {" ": {"dev": {}}}}`,
			want: "group name cannot be blank",
		},
		{
			name: "blank profile",
			data: `{"$groups": {"api": {" ": {}}}}`,
			want: "profile name",
		},
		{
			name: "multiple profiles need default",
			data: `{"$groups": {"api": {"dev": {}, "prod": {}}}}`,
			want: `requires "$default"`,
		},
		{
			name: "unknown default",
			data: `{"$groups": {"api": {"$default": "prod", "dev": {}}}}`,
			want: "does not exist",
		},
		{
			name: "default must be string",
			data: `{"$groups": {"api": {"$default": 1, "dev": {}}}}`,
			want: "must be a string",
		},
		{
			name: "group must be object",
			data: `{"$groups": {"api": "dev"}}`,
			want: "must be an object",
		},
		{
			name: "profile must be object",
			data: `{"$groups": {"api": {"dev": "value"}}}`,
			want: "must be an object",
		},
		{
			name: "null profile must be object",
			data: `{"$groups": {"api": {"dev": null}}}`,
			want: "must be an object",
		},
		{
			name: "group cannot contain equals",
			data: `{"$groups": {"api=v1": {"dev": {}}}}`,
			want: "cannot contain '='",
		},
		{
			name: "reserved profile ignoring case",
			data: `{"$groups": {"api": {" $ShArEd ": {}}}}`,
			want: "profile name",
		},
		{
			name: "shared must be object",
			data: `{"$shared": "secret", "$groups": {"api": {"dev": {}}}}`,
			want: `"$shared" must be an object`,
		},
		{
			name: "null shared must be object",
			data: `{"$shared": null, "$groups": {"api": {"dev": {}}}}`,
			want: `"$shared" must be an object`,
		},
		{
			name: "empty groups object",
			data: `{"$shared": {"region": "eu"}, "$groups": {}}`,
			want: "require a group",
		},
		{
			name: "group with only default",
			data: `{"$groups": {"api": {"$default": "dev"}}}`,
			want: "requires a profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCatalog([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestFlatCatalogValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "duplicate exact",
			data: `{"dev": {}, "dev": {}}`,
			want: "duplicate environment names",
		},
		{
			name: "duplicate ignoring case and space",
			data: `{"dev": {}, " DEV ": {}}`,
			want: "duplicate environment names",
		},
		{
			name: "blank name",
			data: `{" ": {}}`,
			want: "cannot be blank",
		},
		{
			name: "reserved default",
			data: `{"$default": {}}`,
			want: "reserved",
		},
		{
			name: "environment must be object",
			data: `{"dev": "oops"}`,
			want: `"dev" must be an object`,
		},
		{
			name: "array environment must be object",
			data: `{"dev": ["oops"]}`,
			want: `"dev" must be an object`,
		},
		{
			name: "null environment must be object",
			data: `{"dev": null}`,
			want: `"dev" must be an object`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCatalog([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestGroupedDefaultsAndOverrides(t *testing.T) {
	cat, err := parseCatalog([]byte(`{
		"$shared": {"TOKEN": "shared", "meta": {"region": "eu"}},
		"$groups": {
			"app": {
				"$DEFAULT": "DEV APP 1",
				"dev app 1": {"token": "app", "url": "one"},
				"dev app 2": {"token": "other", "service": {"url": "two"}}
			},
			"API": {"Prod": {"host": "api"}}
		}
	}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}

	sel := cat.DefaultSelection()
	if got, want := sel.Groups(), map[string]string{
		"API": "Prod",
		"app": "dev app 1",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default selection = %#v, want %#v", got, want)
	}

	sel, err = cat.Select("", map[string]string{"APP": "DEV APP 2"})
	if err != nil {
		t.Fatalf("partial selection: %v", err)
	}
	env, err := cat.Resolve(sel)
	if err != nil {
		t.Fatalf("resolve selection: %v", err)
	}
	if got, want := env.Label(), "API=Prod, app=dev app 2"; got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
	values := env.Values()
	if values["token"] != "other" || values["meta.region"] != "eu" ||
		values["service.url"] != "two" || values["host"] != "api" {
		t.Fatalf("resolved values = %#v", values)
	}
	if _, ok := values["TOKEN"]; ok {
		t.Fatalf("group override should replace shared key case-insensitively: %#v", values)
	}
}

func TestGroupedProfilesMayReuseVariablesWithinGroup(t *testing.T) {
	_, err := parseCatalog([]byte(`{
		"$groups": {
			"api": {
				"$default": "dev",
				"dev": {"token": "one"},
				"prod": {"TOKEN": "two"}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("same-group keys should be valid: %v", err)
	}
}

func TestGroupedCatalogRejectsCrossGroupCollisionWithoutValues(t *testing.T) {
	const secret = "never-print-this"
	data := `{
		"$groups": {
			"api": {"dev": {"TOKEN": "` + secret + `"}},
			"auth": {"dev": {"token": "other-secret"}}
		}
	}`
	_, err := parseCatalog([]byte(data))
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), `"TOKEN"`) {
		t.Fatalf("error = %v, want variable name", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "other-secret") {
		t.Fatalf("collision error exposed a value: %v", err)
	}
}

func TestGroupedScopeIsDeterministic(t *testing.T) {
	a, err := NewGroupedCatalog(nil, []Group{
		{Name: "app", Profiles: EnvironmentSet{"Dev App": {}}},
		{Name: "API", Profiles: EnvironmentSet{"DEV": {}}},
	})
	if err != nil {
		t.Fatalf("new first catalog: %v", err)
	}
	b, err := NewGroupedCatalog(nil, []Group{
		{Name: "api", Profiles: EnvironmentSet{"dev": {}}},
		{Name: "APP", Profiles: EnvironmentSet{"dev app": {}}},
	})
	if err != nil {
		t.Fatalf("new second catalog: %v", err)
	}
	ae, err := a.Resolve(a.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve first: %v", err)
	}
	be, err := b.Resolve(b.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if ae.Scope() != be.Scope() {
		t.Fatalf("scopes differ: %q != %q", ae.Scope(), be.Scope())
	}
}

func TestGroupedScopeExcludesValues(t *testing.T) {
	first, err := NewGroupedCatalog(
		map[string]string{"secret": "one"},
		[]Group{{Name: "auth", Profiles: EnvironmentSet{"dev": {"token": "first"}}}},
	)
	if err != nil {
		t.Fatalf("first catalog: %v", err)
	}
	second, err := NewGroupedCatalog(
		map[string]string{"secret": "two"},
		[]Group{{Name: "auth", Profiles: EnvironmentSet{"dev": {"token": "second"}}}},
	)
	if err != nil {
		t.Fatalf("second catalog: %v", err)
	}
	a, _ := first.Resolve(first.DefaultSelection())
	b, _ := second.Resolve(second.DefaultSelection())
	if a.Scope() != b.Scope() {
		t.Fatalf("scope should depend only on selection: %q != %q", a.Scope(), b.Scope())
	}
	for _, secret := range []string{"one", "two", "first", "second"} {
		if strings.Contains(a.Scope(), secret) || strings.Contains(b.Scope(), secret) {
			t.Fatalf("scope exposed value %q", secret)
		}
	}
}

func TestResolveGroupedTargetsHoldsOtherGroupsFixed(t *testing.T) {
	cat, err := NewGroupedCatalog(nil, []Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: EnvironmentSet{
				"dev":  {"api.url": "dev"},
				"prod": {"api.url": "prod"},
			},
		},
		{
			Name:    "auth",
			Default: "personal",
			Profiles: EnvironmentSet{
				"personal": {"token": "p"},
				"ci":       {"token": "c"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	base, err := cat.Select("", map[string]string{"auth": "ci"})
	if err != nil {
		t.Fatalf("base selection: %v", err)
	}
	targets, err := cat.CompareTargets(base, "API", "prod", []string{"dev", "prod"})
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}
	for _, target := range targets {
		if got := target.Env.Selection().Groups()["auth"]; got != "ci" {
			t.Fatalf("auth profile changed to %q in %q", got, target.Env.Label())
		}
	}
	if got := targets[1].Name(); got != "prod" {
		t.Fatalf("target name = %q, want prod", got)
	}
	if _, err := cat.CompareTargets(base, "api", "staging", []string{"dev", "prod"}); err == nil {
		t.Fatalf("expected unknown baseline to be rejected")
	}
	if _, err := cat.CompareTargets(base, "missing", "", []string{"dev", "prod"}); err == nil {
		t.Fatalf("expected unknown compare group to be rejected")
	}
}

func FuzzParseCatalog(f *testing.F) {
	for _, seed := range []string{
		`{}`,
		`{"dev":{"url":"https://example.com"}}`,
		`{"$groups":{"api":{"dev":{"url":"https://example.com"}}}}`,
		`{"$shared":{"secret":"x"},"$groups":{"api":{"dev":{}}}}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cat, err := parseCatalog([]byte(raw))
		if err != nil {
			return
		}
		if _, err := cat.Resolve(cat.DefaultSelection()); err != nil {
			t.Fatalf("parsed catalog did not resolve its default: %v", err)
		}
	})
}

// A reader that keys environment names refuses a map holding two forms of one
// name, so Resolve collapses them. Which form wins has to be the same on every
// run, not whichever the map order produced.
func TestResolveCollapsesEquivalentNames(t *testing.T) {
	for range 16 {
		cat, err := NewCatalog(EnvironmentSet{"dev": {" token": "a", "token": "b", "TOKEN": "c"}})
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		sel, err := cat.Select("dev", nil)
		if err != nil {
			t.Fatalf("select: %v", err)
		}
		env, err := cat.Resolve(sel)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		values := env.Values()
		if len(values) != 1 {
			t.Fatalf("Values() = %#v, want one entry per name", values)
		}
		if got := values["token"]; got != "b" {
			t.Fatalf("Values()[token] = %q, want %q on every run", got, "b")
		}
	}
}

// Group layers merge the same way: a later layer wins, and two forms of one
// name inside a layer resolve without depending on map order.
func TestMergeValuesIsDeterministic(t *testing.T) {
	shared := map[string]string{"Region": "eu", "region": "us"}
	over := map[string]string{" REGION ": "apac"}

	for range 16 {
		got := mergeValues(shared, nil)
		if len(got) != 1 || got["region"] != "us" {
			t.Fatalf("mergeValues(shared) = %#v, want region=us on every run", got)
		}
		if got := mergeValues(shared, over); len(got) != 1 || got["REGION"] != "apac" {
			t.Fatalf("mergeValues(shared, over) = %#v, want the later layer to win", got)
		}
	}
}

func TestMergePrivateGrouped(t *testing.T) {
	pub, err := NewGroupedCatalog(nil, []Group{{
		Name:    "api",
		Default: "dev",
		Profiles: EnvironmentSet{
			"dev":  {"host": "localhost", "key": "public"},
			"prod": {"host": "example.com", "key": "public"},
		},
	}})
	if err != nil {
		t.Fatalf("public grouped catalog: %v", err)
	}
	priv, err := NewGroupedCatalog(nil, []Group{{
		Name:    "api",
		Default: "dev",
		Profiles: EnvironmentSet{
			"dev": {"key": "private"},
		},
	}})
	if err != nil {
		t.Fatalf("private grouped catalog: %v", err)
	}

	merged := pub.mergePrivate(priv)
	if merged.Empty() {
		t.Fatal("merge should not empty the catalog")
	}
	g, ok := merged.findGroup("api")
	if !ok {
		t.Fatal("group api missing")
	}
	if got := g.Profiles["dev"]["key"]; got != "private" {
		t.Fatalf("dev key = %q, want private", got)
	}
	if got := g.Profiles["dev"]["host"]; got != "localhost" {
		t.Fatalf("dev host = %q, want localhost", got)
	}
	if got := g.Profiles["prod"]["key"]; got != "public" {
		t.Fatalf("prod key = %q, want public", got)
	}
}

func TestMergePrivateShapeMismatchKeepsPublic(t *testing.T) {
	pub, err := NewCatalog(EnvironmentSet{"dev": {"host": "a"}})
	if err != nil {
		t.Fatalf("flat catalog: %v", err)
	}
	priv, err := NewGroupedCatalog(nil, []Group{{
		Name:     "api",
		Default:  "dev",
		Profiles: EnvironmentSet{"dev": {"key": "p"}},
	}})
	if err != nil {
		t.Fatalf("grouped catalog: %v", err)
	}

	merged := pub.mergePrivate(priv)
	if merged.Grouped() {
		t.Fatal("shape mismatch should not turn a flat catalog grouped")
	}
	dev, ok := merged.findEnv("dev")
	if !ok || dev.values["host"] != "a" {
		t.Fatalf("flat values not preserved: %+v", dev)
	}
	if _, ok := dev.values["key"]; ok {
		t.Fatal("private flat overlay should not apply to a grouped mismatch")
	}
}
