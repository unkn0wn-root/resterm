package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogIncludesDefaultAndUserThemes(t *testing.T) {
	dir := t.TempDir()

	tomlContent := []byte(`
[metadata]
name = "Oceanic"
author = "QA"

[styles.header]
foreground = "#ddeeff"

[colors]
pane_active_foreground = "#335577"
`)
	if err := os.WriteFile(filepath.Join(dir, "oceanic.toml"), tomlContent, 0o644); err != nil {
		t.Fatalf("write toml theme: %v", err)
	}

	jsonContent := []byte(`{
  "metadata": {
    "name": "Oceanic",
    "author": "QA"
  },
  "colors": {
    "pane_border_focus_file": "#ff9900"
  },
  "editor_metadata": {
    "comment_marker": "#123123"
  }
}`)
	if err := os.WriteFile(filepath.Join(dir, "sunset.json"), jsonContent, 0o644); err != nil {
		t.Fatalf("write json theme: %v", err)
	}

	catalog, err := LoadCatalog([]string{dir})
	if err != nil {
		t.Fatalf("LoadCatalog returned error: %v", err)
	}

	if _, ok := catalog.Get("default"); !ok {
		t.Fatalf("expected default theme to be present")
	}

	oceanic, ok := catalog.Get("oceanic")
	if !ok {
		t.Fatalf("expected oceanic theme to load")
	}
	if oceanic.Metadata.Author != "QA" {
		t.Fatalf("expected author QA, got %q", oceanic.Metadata.Author)
	}
	if oceanic.Theme.PaneActiveForeground != "#335577" {
		t.Fatalf(
			"expected pane active foreground override, got %q",
			oceanic.Theme.PaneActiveForeground,
		)
	}

	duplicate, ok := catalog.Get("oceanic-1")
	if !ok {
		t.Fatalf("expected duplicate slug to be uniquified")
	}
	if duplicate.Theme.PaneBorderFocusFile != "#ff9900" {
		t.Fatalf("expected JSON theme color override, got %q", duplicate.Theme.PaneBorderFocusFile)
	}
	if duplicate.Theme.EditorMetadata.CommentMarker != "#123123" {
		t.Fatalf(
			"expected metadata override from JSON theme, got %q",
			duplicate.Theme.EditorMetadata.CommentMarker,
		)
	}
}

func TestLoadCatalogHandlesMissingDirectory(t *testing.T) {
	catalog, err := LoadCatalog([]string{"/nonexistent/path"})
	if err != nil {
		t.Fatalf("LoadCatalog should not error on missing directories: %v", err)
	}
	if _, ok := catalog.Get("default"); !ok {
		t.Fatalf("expected default theme even when directories are missing")
	}
	if len(catalog.All()) != 1 {
		t.Fatalf("expected only default theme, got %d", len(catalog.All()))
	}
}
