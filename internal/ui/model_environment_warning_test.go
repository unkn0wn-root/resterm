package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func envEntry(name, path string) files.Entry {
	return files.Entry{Name: name, Path: path, Kind: files.KindEnv}
}

func TestInactiveEnvStatus(t *testing.T) {
	active := filepath.Join("/ws", "resterm.env.json")
	nested := []files.Entry{
		envEntry("resterm.env.json", active),
		envEntry(filepath.Join("a", "resterm.env.json"), filepath.Join("/ws", "a", "resterm.env.json")),
		envEntry(filepath.Join("b", "resterm.env.json"), filepath.Join("/ws", "b", "resterm.env.json")),
	}

	for _, tc := range []struct {
		name      string
		entries   []files.Entry
		active    string
		recursive bool
		want      string
	}{
		{
			name:      "two nested files",
			entries:   nested,
			active:    active,
			recursive: true,
			want:      "2 environment files are inactive. This workspace uses resterm.env.json",
		},
		{
			name:      "one nested file names it",
			entries:   nested[:2],
			active:    active,
			recursive: true,
			want:      filepath.Join("a", "resterm.env.json") + " is inactive. This workspace uses resterm.env.json",
		},
		{
			name:      "nothing loaded",
			entries:   nested[1:],
			active:    "",
			recursive: true,
			want:      "2 environment files are inactive. No environment file was loaded",
		},
		{
			name:      "only the active file",
			entries:   nested[:1],
			active:    active,
			recursive: true,
		},
		{
			// Without recursion the navigator never lists the nested files, so
			// there is nothing to be surprised by.
			name:      "not recursive",
			entries:   nested,
			active:    active,
			recursive: false,
		},
		{
			name:      "request files are ignored",
			entries:   []files.Entry{{Name: "a/req.http", Path: "/ws/a/req.http", Kind: files.KindRequest}},
			active:    active,
			recursive: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := inactiveEnvStatus(tc.entries, tc.active, tc.recursive)
			if got.text != tc.want {
				t.Fatalf("text = %q, want %q", got.text, tc.want)
			}
			if tc.want != "" && got.level != statusWarn {
				t.Fatalf("level = %v, want warn", got.level)
			}
		})
	}
}

// The scan walks the workspace itself, so these cases use real files.
func TestUndiscoveredEnvStatus(t *testing.T) {
	loaded := testCatalog(vars.EnvironmentSet{"dev": {"a": "1"}})

	for _, tc := range []struct {
		name      string
		files     []string
		recursive bool
		cat       vars.Catalog
		active    string
		want      string
	}{
		{
			name:  "one candidate names it",
			files: []string{"dev.env.json"},
			want:  "No environment file was discovered. Load dev.env.json with --env-file",
		},
		{
			name:      "several candidates are counted",
			files:     []string{"dev.env.json", "prod.env.json", "a/test.env.json"},
			recursive: true,
			want:      "No environment file was discovered. Load one of 3 *.env.json files with --env-file",
		},
		{
			name:  "nested candidates need a recursive workspace",
			files: []string{"a/dev.env.json"},
		},
		{
			// Sitting beside a discovered file makes them a deliberate choice
			// for --env-file rather than an accident.
			name:   "an environment is already loaded",
			files:  []string{"dev.env.json", "resterm.env.json"},
			cat:    loaded,
			active: "resterm.env.json",
		},
		{
			name:   "explicit env file",
			files:  []string{"dev.env.json"},
			cat:    loaded,
			active: "dev.env.json",
		},
		{
			name:  "plain json is not a candidate",
			files: []string{"package.json", "mock-user.json"},
		},
		{
			name:  "the discoverable names are not candidates",
			files: []string{"resterm.env.json", "rest-client.env.json"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, rel := range tc.files {
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"e":{"a":"1"}}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			active := ""
			if tc.active != "" {
				active = filepath.Join(root, tc.active)
			}

			got := undiscoveredEnvStatus(workspace{
				root:      root,
				recursive: tc.recursive,
				cat:       tc.cat,
				envFile:   active,
			})
			if got.text != tc.want {
				t.Fatalf("text = %q, want %q", got.text, tc.want)
			}
			if tc.want != "" && got.level != statusWarn {
				t.Fatalf("level = %v, want warn", got.level)
			}
		})
	}
}

func writeWorkspace(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func modelInWorkspace(t *testing.T, root string, recursive bool) Model {
	t.Helper()
	cat, envFile, err := vars.Discover(root)
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	sel, err := cat.Select("", nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	return New(Config{
		Env:           vars.Config{Catalog: cat, Selection: sel, File: envFile},
		WorkspaceRoot: root,
		Recursive:     recursive,
	})
}

func TestNestedEnvironmentFileWarnsAtStartup(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json":   `{"rootenv":{"who":"ROOT"}}`,
		"a/resterm.env.json": `{"aenv":{"who":"DIR-A"}}`,
		"a/req.http":         "### who\n# @name whoa\nGET http://example.test/{{who}}\n",
		"root.http":          "### who\n# @name whoroot\nGET http://example.test/{{who}}\n",
	})
	m := modelInWorkspace(t, root, true)

	if got := m.statusMessage.text; !strings.Contains(got, "inactive") {
		t.Fatalf("status = %q, want a warning about the nested environment file", got)
	}
	if got := m.statusMessage.level; got != statusWarn {
		t.Fatalf("level = %v, want warn", got)
	}

	// The warning is accurate because opening the nested file keeps the root
	// catalog.
	m.openFile(filepath.Join(root, "a", "req.http"))
	if got := m.ws.active.Values()["who"]; got != "ROOT" {
		t.Fatalf("who = %q, want the root environment to stay active", got)
	}
}
