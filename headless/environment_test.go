package headless

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildOptionsGroupedEnvironment(t *testing.T) {
	dir := t.TempDir()
	opt := Options{
		Source: Source{Path: filepath.Join(dir, "api.http")},
		Environment: EnvironmentOptions{
			FilePath: filepath.Join(dir, "missing.json"),
			Grouped: &GroupedEnvironmentSet{
				Shared: map[string]string{"region": "eu"},
				Groups: EnvironmentGroups{
					"api": {
						Default: "dev",
						Profiles: EnvironmentSet{
							"dev":  {"api.url": "dev"},
							"prod": {"api.url": "prod"},
						},
					},
					"app": {
						Default: "dev app 1",
						Profiles: EnvironmentSet{
							"dev app 1": {"app.url": "one"},
							"dev app 2": {"app.url": "two"},
						},
					},
				},
			},
			Selection: EnvironmentSelection{"app": "dev app 2"},
		},
		Compare: CompareOptions{
			Targets: []string{"dev", "prod"},
			Base:    "prod",
			Group:   "api",
		},
	}

	got, err := buildOptions(opt)
	if err != nil {
		t.Fatalf("build options: %v", err)
	}
	if got.EnvironmentFile != "" {
		t.Fatalf("injected definitions should ignore filePath, got %q", got.EnvironmentFile)
	}
	env, err := got.Catalog.Resolve(got.Selection)
	if err != nil {
		t.Fatalf("resolve environment: %v", err)
	}
	if got, want := env.Label(), "api=dev, app=dev app 2"; got != want {
		t.Fatalf("environment = %q, want %q", got, want)
	}
	if got, want := env.Selection().Groups(), map[string]string{
		"api": "dev",
		"app": "dev app 2",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection = %#v, want %#v", got, want)
	}
}

func TestBuildOptionsEnvironmentConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api.http")
	grouped := &GroupedEnvironmentSet{
		Groups: EnvironmentGroups{
			"api": {
				Default: "dev",
				Profiles: EnvironmentSet{
					"dev":  {},
					"prod": {},
				},
			},
		},
	}

	tests := []struct {
		name string
		env  EnvironmentOptions
		cmp  CompareOptions
		want string
	}{
		{
			name: "set and grouped",
			env: EnvironmentOptions{
				Set:     EnvironmentSet{"dev": {}},
				Grouped: grouped,
			},
			want: "environment.set cannot be combined",
		},
		{
			name: "name and selection",
			env: EnvironmentOptions{
				Grouped:   grouped,
				Name:      "dev",
				Selection: EnvironmentSelection{"api": "dev"},
			},
			want: "environment.name cannot be combined",
		},
		{
			name: "flat selection",
			env: EnvironmentOptions{
				Set:       EnvironmentSet{"dev": {}},
				Selection: EnvironmentSelection{"api": "dev"},
			},
			want: "requires grouped environments",
		},
		{
			name: "grouped name",
			env: EnvironmentOptions{
				Grouped: grouped,
				Name:    "dev",
			},
			want: "cannot be combined with grouped environments",
		},
		{
			name: "grouped compare without group",
			env:  EnvironmentOptions{Grouped: grouped},
			cmp:  CompareOptions{Targets: []string{"dev", "prod"}},
			want: "grouped compare requires a group",
		},
		{
			name: "unknown compare profile",
			env:  EnvironmentOptions{Grouped: grouped},
			cmp: CompareOptions{
				Targets: []string{"dev", "missing"},
				Group:   "api",
			},
			want: "unknown profile",
		},
		{
			name: "invalid compare baseline",
			env:  EnvironmentOptions{Grouped: grouped},
			cmp: CompareOptions{
				Targets: []string{"dev", "prod"},
				Base:    "missing",
				Group:   "api",
			},
			want: "must match a compare target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildOptions(Options{
				Source:      Source{Path: path},
				Environment: tt.env,
				Compare:     tt.cmp,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
			if !IsUsageError(err) {
				t.Fatalf("error = %v, want UsageError", err)
			}
		})
	}
}

func TestBuildFailsForInvalidEnvironmentFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "api.http")
	envFile := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(source, []byte("GET https://example.com\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(
		envFile,
		[]byte(`{"$groups":{"api":{"dev":{"token":"a"}},"auth":{"dev":{"TOKEN":"b"}}}}`),
		0o600,
	); err != nil {
		t.Fatalf("write environment: %v", err)
	}

	_, err := Build(Options{
		Source: Source{Path: source},
		Environment: EnvironmentOptions{
			FilePath: envFile,
		},
	})
	if err == nil {
		t.Fatal("expected invalid environment file to fail Build")
	}
	if !IsUsageError(err) || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("error = %v, want usage collision error", err)
	}
}
