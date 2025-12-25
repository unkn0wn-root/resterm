package config

import "testing"

func TestNormaliseLayoutSettingsDefaultsAndBounds(t *testing.T) {
	layout := NormaliseLayoutSettings(LayoutSettings{})
	if layout.SidebarWidth != LayoutSidebarWidthDefault {
		t.Fatalf(
			"expected sidebar width default %v, got %v",
			LayoutSidebarWidthDefault,
			layout.SidebarWidth,
		)
	}
	if layout.EditorSplit != LayoutEditorSplitDefault {
		t.Fatalf(
			"expected editor split default %v, got %v",
			LayoutEditorSplitDefault,
			layout.EditorSplit,
		)
	}
	if layout.ResponseSplitRatio != LayoutResponseRatioDefault {
		t.Fatalf(
			"expected response split ratio default %v, got %v",
			LayoutResponseRatioDefault,
			layout.ResponseSplitRatio,
		)
	}
	if layout.MainSplit != LayoutMainSplitVertical {
		t.Fatalf("expected main split vertical, got %v", layout.MainSplit)
	}
	if layout.ResponseOrientation != LayoutResponseOrientationVertical {
		t.Fatalf("expected response orientation vertical, got %v", layout.ResponseOrientation)
	}
}

func TestNormaliseLayoutSettingsClampsValues(t *testing.T) {
	raw := LayoutSettings{
		SidebarWidth:        0,
		EditorSplit:         1.2,
		MainSplit:           "SIDEWAYS",
		ResponseSplit:       true,
		ResponseSplitRatio:  0.01,
		ResponseOrientation: "Diagonal",
	}
	layout := NormaliseLayoutSettings(raw)
	if layout.SidebarWidth != LayoutSidebarWidthDefault {
		t.Fatalf(
			"expected sidebar width default %v, got %v",
			LayoutSidebarWidthDefault,
			layout.SidebarWidth,
		)
	}
	if layout.EditorSplit != LayoutEditorSplitMax {
		t.Fatalf(
			"expected editor split clamped to %v, got %v",
			LayoutEditorSplitMax,
			layout.EditorSplit,
		)
	}
	if layout.MainSplit != LayoutMainSplitVertical {
		t.Fatalf("expected main split fallback to vertical, got %v", layout.MainSplit)
	}
	if !layout.ResponseSplit {
		t.Fatalf("expected response split to remain true")
	}
	if layout.ResponseSplitRatio != LayoutResponseRatioMin {
		t.Fatalf(
			"expected response ratio clamped to %v, got %v",
			LayoutResponseRatioMin,
			layout.ResponseSplitRatio,
		)
	}
	if layout.ResponseOrientation != LayoutResponseOrientationVertical {
		t.Fatalf(
			"expected response orientation fallback to vertical, got %v",
			layout.ResponseOrientation,
		)
	}
}

func TestNormaliseMainSplitHonoursExplicitVertical(t *testing.T) {
	split := normaliseMainSplit(LayoutMainSplitVertical, LayoutMainSplitHorizontal)
	if split != LayoutMainSplitVertical {
		t.Fatalf("expected explicit vertical to be preserved, got %v", split)
	}
}

func TestNormaliseResponseOrientationHonoursExplicitVertical(t *testing.T) {
	orientation := normaliseResponseOrientation(
		LayoutResponseOrientationVertical,
		LayoutResponseOrientationHorizontal,
	)
	if orientation != LayoutResponseOrientationVertical {
		t.Fatalf("expected explicit vertical to be preserved, got %v", orientation)
	}
}
