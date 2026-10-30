package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestEditorDiagnosticsSettings(t *testing.T) {
	for _, tt := range []struct {
		format SettingsFormat
		source string
		want   bool
	}{
		{SettingsFormatJSON, `{}`, true},
		{SettingsFormatJSON, `{"editor":{"diagnostics":false}}`, false},
		{SettingsFormatJSON, `{"editor":{"diagnostics":true}}`, true},
		{SettingsFormatTOML, "", true},
		{SettingsFormatTOML, "[editor]\ndiagnostics = false", false},
		{SettingsFormatTOML, "[editor]\ndiagnostics = true", true},
	} {
		settings, err := decodeSettings([]byte(tt.source), tt.format)
		if err != nil || settings.Editor.DiagnosticsEnabled() != tt.want {
			t.Fatalf("decode %q: %+v %v", tt.source, settings, err)
		}
		settings.DefaultTheme = "oceanic"
		path := filepath.Join(t.TempDir(), "settings."+string(tt.format))
		if err := SaveSettings(settings, SettingsHandle{Path: path, Format: tt.format}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeSettings(data, tt.format)
		if err != nil || got.Editor.DiagnosticsEnabled() != tt.want || got.DefaultTheme != "oceanic" {
			t.Fatalf("round trip: %+v %v", got, err)
		}
	}
}

func TestClipboardSettings(t *testing.T) {
	for _, tt := range []struct {
		format SettingsFormat
		source string
		text   string
		want   int64
	}{
		{SettingsFormatJSON, `{}`, "", 0},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"8MiB"}}`, "8MiB", 8 << 20},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"16 KiB"}}`, "16 KiB", 16 << 10},
		{SettingsFormatTOML, "", "", 0},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '8MiB'", "8MiB", 8 << 20},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '16 KiB'", "16 KiB", 16 << 10},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = 1", "1", 1},
	} {
		settings, err := decodeSettings([]byte(tt.source), tt.format)
		if err != nil || settings.Clipboard.OSC52LimitBytes() != tt.want {
			t.Fatalf("decode %q: %+v %v", tt.source, settings, err)
		}
		settings.DefaultTheme = "oceanic"
		path := filepath.Join(t.TempDir(), "settings."+string(tt.format))
		if err := SaveSettings(settings, SettingsHandle{Path: path, Format: tt.format}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if tt.text == "" && strings.Contains(string(data), "clipboard") {
			t.Fatalf("saved %q, want no clipboard", data)
		}
		if !strings.Contains(string(data), tt.text) {
			t.Fatalf("saved %q, want %q", data, tt.text)
		}
		got, err := decodeSettings(data, tt.format)
		if err != nil || got.Clipboard.OSC52LimitBytes() != tt.want || got.DefaultTheme != "oceanic" {
			t.Fatalf("round trip: %+v %v", got, err)
		}
	}
}

func TestClipboardSettingsRejectInvalidLimit(t *testing.T) {
	for _, tt := range []struct {
		format SettingsFormat
		source string
	}{
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":""}}`},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"0"}}`},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"0KiB"}}`},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"-1"}}`},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":"8MiBs"}}`},
		{SettingsFormatJSON, `{"clipboard":{"osc52_limit":1024}}`},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = ''"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '0'"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = 0"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '0KiB'"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '-1'"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = -1"},
		{SettingsFormatTOML, "[clipboard]\nosc52_limit = '8MiBs'"},
	} {
		if settings, err := decodeSettings([]byte(tt.source), tt.format); err == nil {
			t.Fatalf("decode %q = %+v, want an error", tt.source, settings)
		}
	}
}

func TestSaveSettingsRefusesAfterLoadError(t *testing.T) {
	tests := []struct {
		name string
		file string
		data string
	}{
		{name: "broken toml", file: "settings.toml", data: "default_theme = ["},
		{name: "broken json", file: "settings.json", data: "{"},
		{name: "unreadable toml", file: "settings.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("RESTERM_CONFIG_DIR", dir)
			path := filepath.Join(dir, tt.file)
			var err error
			if tt.data == "" {
				err = os.Mkdir(path, 0o755)
			} else {
				err = os.WriteFile(path, []byte(tt.data), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}

			_, handle, loadErr := LoadSettings()
			if loadErr == nil {
				t.Fatal("LoadSettings() error = nil, want an error")
			}
			saveErr := SaveSettings(Settings{DefaultTheme: "oceanic"}, handle)
			if !errors.Is(saveErr, loadErr) {
				t.Fatalf("SaveSettings() = %v, want it to wrap %v", saveErr, loadErr)
			}

			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != tt.file {
				t.Fatalf("config dir has %v, %v, want only %s", entries, err, tt.file)
			}
			if tt.data == "" {
				return
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != tt.data {
				t.Fatalf("%s = %q, %v, want %q", tt.file, got, err, tt.data)
			}
		})
	}
}

func TestSaveSettingsThroughLoadedHandle(t *testing.T) {
	tests := []struct {
		file string
		data string
	}{
		{file: "settings.toml", data: "default_theme = 'sunset'\n\n[clipboard]\nosc52_limit = '8MiB'"},
		{file: "settings.json", data: `{"default_theme":"sunset","clipboard":{"osc52_limit":"8MiB"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("RESTERM_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}

			settings, handle, err := LoadSettings()
			if err != nil {
				t.Fatalf("LoadSettings() error = %v", err)
			}
			settings.DefaultTheme = "oceanic"
			if err := SaveSettings(settings, handle); err != nil {
				t.Fatalf("SaveSettings() = %v, want nil", err)
			}

			got, _, err := LoadSettings()
			if err != nil || got.DefaultTheme != "oceanic" || got.Clipboard.OSC52LimitBytes() != 8<<20 {
				t.Fatalf("reload = %+v, %v, want theme oceanic and limit %d", got, err, 8<<20)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != tt.file {
				t.Fatalf("config dir has %v, %v, want only %s", entries, err, tt.file)
			}
		})
	}
}

func TestLoadSettingsBesideValidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)
	path := filepath.Join(dir, "settings.toml")
	jsonPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("default_theme = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, []byte(`{"default_theme":"sunset"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, handle, loadErr := LoadSettings()
	if loadErr == nil {
		t.Fatal("LoadSettings() with a broken settings.toml error = nil, want a parse error")
	}
	if err := SaveSettings(Settings{DefaultTheme: "oceanic"}, handle); !errors.Is(err, loadErr) {
		t.Fatalf("SaveSettings() = %v, want it to wrap %v", err, loadErr)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	settings, handle, err := LoadSettings()
	if err != nil || settings.DefaultTheme != "sunset" || handle.Path != jsonPath {
		t.Fatalf("LoadSettings() = %+v, %+v, %v, want settings.json", settings, handle, err)
	}
	settings.DefaultTheme = "oceanic"
	if err := SaveSettings(settings, handle); err != nil {
		t.Fatalf("SaveSettings() = %v, want nil", err)
	}
	got, _, err := LoadSettings()
	if err != nil || got.DefaultTheme != "oceanic" {
		t.Fatalf("reload = %+v, %v, want theme oceanic", got, err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("settings.toml has %v, %v, want an empty directory", entries, err)
	}
}
