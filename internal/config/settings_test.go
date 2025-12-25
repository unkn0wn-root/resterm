package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsReturnsDefaultHandleWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)

	settings, handle, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings returned error: %v", err)
	}
	expectedPath := filepath.Join(dir, "settings.toml")
	if handle.Path != expectedPath {
		t.Fatalf("expected handle path %q, got %q", expectedPath, handle.Path)
	}
	if handle.Format != SettingsFormatTOML {
		t.Fatalf("expected format %q, got %q", SettingsFormatTOML, handle.Format)
	}
	if settings.Layout.SidebarWidth != LayoutSidebarWidthDefault {
		t.Fatalf(
			"expected default sidebar width %v, got %v",
			LayoutSidebarWidthDefault,
			settings.Layout.SidebarWidth,
		)
	}
	if settings.DefaultTheme != "" {
		t.Fatalf("expected empty default theme, got %q", settings.DefaultTheme)
	}
}

func TestSaveAndLoadSettingsTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)

	want := Settings{DefaultTheme: "oceanic"}
	if err := SaveSettings(want, SettingsHandle{}); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	got, handle, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if got.DefaultTheme != want.DefaultTheme {
		t.Fatalf("expected theme %q, got %q", want.DefaultTheme, got.DefaultTheme)
	}
	if handle.Format != SettingsFormatTOML {
		t.Fatalf("expected format %q after save, got %q", SettingsFormatTOML, handle.Format)
	}
}

func TestLoadSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)

	payload := Settings{DefaultTheme: "sunset"}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write json settings: %v", err)
	}

	got, handle, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if got.DefaultTheme != payload.DefaultTheme {
		t.Fatalf("expected theme %q, got %q", payload.DefaultTheme, got.DefaultTheme)
	}
	if handle.Format != SettingsFormatJSON {
		t.Fatalf("expected json format, got %q", handle.Format)
	}
	if handle.Path != path {
		t.Fatalf("expected handle path %q, got %q", path, handle.Path)
	}
}
