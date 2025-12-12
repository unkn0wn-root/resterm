package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func TestSaveLayoutSettingsPersistsAndClosesModal(t *testing.T) {
	dir := t.TempDir()
	handle := config.SettingsHandle{
		Path:   filepath.Join(dir, "settings.toml"),
		Format: config.SettingsFormatTOML,
	}
	cfg := Config{
		WorkspaceRoot:  dir,
		Settings:       config.Settings{},
		SettingsHandle: handle,
	}
	model := New(cfg)

	model.sidebarWidth = 0.25
	model.editorSplit = 0.55
	model.mainSplitOrientation = mainSplitHorizontal
	model.responseSplit = true
	model.responseSplitOrientation = responseSplitHorizontal
	model.responseSplitRatio = 0.7
	model.openLayoutSaveModal()

	cmd := model.saveLayoutSettings()
	if cmd == nil {
		t.Fatalf("expected saveLayoutSettings to return a command")
	}
	if model.showLayoutSaveModal {
		t.Fatalf("expected modal to close after saving")
	}

	msg := cmd()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected status message, got %T", msg)
	}
	if status.level != statusSuccess {
		t.Fatalf("expected success status, got %v", status.level)
	}

	data, err := os.ReadFile(handle.Path)
	if err != nil {
		t.Fatalf("expected settings file to be written: %v", err)
	}

	var settings config.Settings
	if err := toml.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to decode settings: %v", err)
	}
	layout := config.NormaliseLayoutSettings(settings.Layout)
	if layout.SidebarWidth != 0.25 {
		t.Fatalf("expected sidebar width 0.25, got %v", layout.SidebarWidth)
	}
	if layout.EditorSplit != 0.55 {
		t.Fatalf("expected editor split 0.55, got %v", layout.EditorSplit)
	}
	if layout.MainSplit != config.LayoutMainSplitHorizontal {
		t.Fatalf("expected main split horizontal, got %v", layout.MainSplit)
	}
	if !layout.ResponseSplit {
		t.Fatalf("expected response split to be enabled")
	}
	if layout.ResponseOrientation != config.LayoutResponseOrientationHorizontal {
		t.Fatalf("expected response orientation horizontal, got %v", layout.ResponseOrientation)
	}
	if layout.ResponseSplitRatio != 0.7 {
		t.Fatalf("expected response split ratio 0.7, got %v", layout.ResponseSplitRatio)
	}
}

func TestSaveLayoutSettingsErrorClosesModal(t *testing.T) {
	dir := t.TempDir()
	handle := config.SettingsHandle{
		Path:   filepath.Join(dir, "settings.toml"),
		Format: config.SettingsFormat("yaml"), // unsupported to force failure
	}
	cfg := Config{
		WorkspaceRoot:  dir,
		Settings:       config.Settings{},
		SettingsHandle: handle,
	}
	model := New(cfg)
	model.openLayoutSaveModal()

	cmd := model.saveLayoutSettings()
	if cmd == nil {
		t.Fatalf("expected saveLayoutSettings to return a command")
	}
	if model.showLayoutSaveModal {
		t.Fatalf("expected modal to close on error")
	}

	msg := cmd()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected status message, got %T", msg)
	}
	if status.level != statusError {
		t.Fatalf("expected error status, got %v", status.level)
	}

	if _, err := os.Stat(handle.Path); err == nil {
		t.Fatalf("expected settings file not to be created on error")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing file, got %v", err)
	}
}
