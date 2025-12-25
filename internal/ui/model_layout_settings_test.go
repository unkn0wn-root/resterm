package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func TestApplyLayoutSettingsFromConfig(t *testing.T) {
	cfg := Config{
		WorkspaceRoot: t.TempDir(),
		Settings: config.Settings{
			Layout: config.LayoutSettings{
				SidebarWidth:        0.25,
				EditorSplit:         0.45,
				MainSplit:           config.LayoutMainSplitHorizontal,
				ResponseSplit:       true,
				ResponseSplitRatio:  0.7,
				ResponseOrientation: config.LayoutResponseOrientationHorizontal,
			},
		},
	}
	m := New(cfg)
	if m.sidebarWidth != 0.25 {
		t.Fatalf("expected sidebar width 0.25, got %v", m.sidebarWidth)
	}
	if m.editorSplit != 0.45 {
		t.Fatalf("expected editor split 0.45, got %v", m.editorSplit)
	}
	if m.mainSplitOrientation != mainSplitHorizontal {
		t.Fatalf("expected horizontal main split, got %v", m.mainSplitOrientation)
	}
	if !m.responseSplit {
		t.Fatalf("expected response split to be enabled")
	}
	if m.responseSplitOrientation != responseSplitHorizontal {
		t.Fatalf("expected response split horizontal, got %v", m.responseSplitOrientation)
	}
	if m.responseSplitRatio != 0.7 {
		t.Fatalf("expected response split ratio 0.7, got %v", m.responseSplitRatio)
	}
}

func TestCurrentLayoutSettingsClampsValues(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	m := New(cfg)
	m.sidebarWidth = 1.0
	m.editorSplit = 0.1
	m.responseSplit = true
	m.responseSplitRatio = 5
	m.mainSplitOrientation = mainSplitHorizontal
	m.responseSplitOrientation = responseSplitHorizontal

	layout := m.currentLayoutSettings()
	if layout.SidebarWidth != config.NormaliseLayoutSettings(
		config.LayoutSettings{SidebarWidth: m.sidebarWidth},
	).SidebarWidth {
		t.Fatalf("expected sidebar width to be normalised, got %v", layout.SidebarWidth)
	}
	if layout.EditorSplit != config.NormaliseLayoutSettings(
		config.LayoutSettings{EditorSplit: m.editorSplit},
	).EditorSplit {
		t.Fatalf("expected editor split to be normalised, got %v", layout.EditorSplit)
	}
	if layout.ResponseSplitRatio != config.NormaliseLayoutSettings(
		config.LayoutSettings{ResponseSplitRatio: m.responseSplitRatio},
	).ResponseSplitRatio {
		t.Fatalf(
			"expected response split ratio to be normalised, got %v",
			layout.ResponseSplitRatio,
		)
	}
	if layout.MainSplit != config.LayoutMainSplitHorizontal {
		t.Fatalf("expected main split horizontal token, got %v", layout.MainSplit)
	}
	if layout.ResponseOrientation != config.LayoutResponseOrientationHorizontal {
		t.Fatalf(
			"expected response orientation horizontal token, got %v",
			layout.ResponseOrientation,
		)
	}
	if !layout.ResponseSplit {
		t.Fatalf("expected response split to remain enabled")
	}
}
