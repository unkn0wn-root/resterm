package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func TestLoadThemeStateResolvesConfiguredTheme(t *testing.T) {
	cfgDir := t.TempDir()
	themeDir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", cfgDir)
	t.Setenv("RESTERM_THEMES_DIR", themeDir)

	if err := os.WriteFile(
		filepath.Join(cfgDir, "settings.toml"),
		[]byte("default_theme = \"daybreak\"\n"),
		0o644,
	); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(themeDir, "daybreak.toml"),
		[]byte(`
[metadata]
name = "Daybreak"
tags = ["light"]

[styles.header_title]
foreground = "#1e40af"
bold = true

[styles.list_item_selected_title]
foreground = "#0f172a"
background = "#bfdbfe"
`),
		0o644,
	); err != nil {
		t.Fatalf("write theme: %v", err)
	}

	st, err := loadThemeState()
	if err != nil {
		t.Fatalf("loadThemeState: %v", err)
	}
	if st.active != "daybreak" {
		t.Fatalf("expected active theme %q, got %q", "daybreak", st.active)
	}
	if st.def.Key != "daybreak" {
		t.Fatalf("expected resolved theme key %q, got %q", "daybreak", st.def.Key)
	}
	if st.def.Appearance().String() != "light" {
		t.Fatalf("expected light appearance, got %q", st.def.Appearance())
	}
	if got := st.def.Theme.HeaderTitle.GetForeground(); got != lipgloss.Color("#1e40af") {
		t.Fatalf("expected themed header title foreground, got %v", got)
	}
	if st.settings.DefaultTheme != "daybreak" {
		t.Fatalf(
			"expected normalized settings theme %q, got %q",
			"daybreak",
			st.settings.DefaultTheme,
		)
	}
}

func TestLoadThemeStateKeepsBrokenSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)
	path := filepath.Join(dir, "settings.toml")
	data := "default_theme = ["
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	st, err := loadThemeState()
	if err == nil {
		t.Fatal("loadThemeState() error = nil, want a settings error")
	}
	if err := config.SaveSettings(st.settings, st.handle); err == nil {
		t.Fatal("SaveSettings() error = nil, want a refusal")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != data {
		t.Fatalf("settings.toml = %q, %v, want %q", got, err, data)
	}
}
