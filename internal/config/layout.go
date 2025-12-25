package config

import "strings"

type LayoutMainSplit string

const (
	LayoutMainSplitVertical   LayoutMainSplit = "vertical"
	LayoutMainSplitHorizontal LayoutMainSplit = "horizontal"
)

type LayoutResponseOrientation string

const (
	LayoutResponseOrientationVertical   LayoutResponseOrientation = "vertical"
	LayoutResponseOrientationHorizontal LayoutResponseOrientation = "horizontal"
)

type LayoutSettings struct {
	SidebarWidth        float64                   `json:"sidebar_width"        toml:"sidebar_width"`
	EditorSplit         float64                   `json:"editor_split"         toml:"editor_split"`
	MainSplit           LayoutMainSplit           `json:"main_split"           toml:"main_split"`
	ResponseSplit       bool                      `json:"response_split"       toml:"response_split"`
	ResponseSplitRatio  float64                   `json:"response_split_ratio" toml:"response_split_ratio"`
	ResponseOrientation LayoutResponseOrientation `json:"response_orientation" toml:"response_orientation"`
}

const (
	LayoutSidebarWidthDefault  = 0.2
	LayoutSidebarWidthMin      = 0.05
	LayoutSidebarWidthMax      = 0.3
	LayoutEditorSplitDefault   = 0.6
	LayoutEditorSplitMin       = 0.3
	LayoutEditorSplitMax       = 0.63
	LayoutResponseRatioDefault = 0.5
	LayoutResponseRatioMin     = 0.1
	LayoutResponseRatioMax     = 0.9
)

func DefaultLayoutSettings() LayoutSettings {
	return LayoutSettings{
		SidebarWidth:        LayoutSidebarWidthDefault,
		EditorSplit:         LayoutEditorSplitDefault,
		MainSplit:           LayoutMainSplitVertical,
		ResponseSplit:       false,
		ResponseSplitRatio:  LayoutResponseRatioDefault,
		ResponseOrientation: LayoutResponseOrientationVertical,
	}
}

func NormaliseLayoutSettings(in LayoutSettings) LayoutSettings {
	layout := DefaultLayoutSettings()
	layout.SidebarWidth = clampFloat(
		in.SidebarWidth,
		LayoutSidebarWidthMin,
		LayoutSidebarWidthMax,
		LayoutSidebarWidthDefault,
	)
	layout.EditorSplit = clampFloat(
		in.EditorSplit,
		LayoutEditorSplitMin,
		LayoutEditorSplitMax,
		LayoutEditorSplitDefault,
	)
	layout.ResponseSplit = in.ResponseSplit
	layout.ResponseSplitRatio = clampFloat(
		in.ResponseSplitRatio,
		LayoutResponseRatioMin,
		LayoutResponseRatioMax,
		LayoutResponseRatioDefault,
	)
	layout.MainSplit = normaliseMainSplit(in.MainSplit, layout.MainSplit)
	layout.ResponseOrientation = normaliseResponseOrientation(
		in.ResponseOrientation,
		layout.ResponseOrientation,
	)
	return layout
}

func normaliseMainSplit(in LayoutMainSplit, def LayoutMainSplit) LayoutMainSplit {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(LayoutMainSplitHorizontal):
		return LayoutMainSplitHorizontal
	case string(LayoutMainSplitVertical):
		return LayoutMainSplitVertical
	default:
		return def
	}
}

func normaliseResponseOrientation(
	in LayoutResponseOrientation,
	def LayoutResponseOrientation,
) LayoutResponseOrientation {
	switch strings.ToLower(strings.TrimSpace(string(in))) {
	case string(LayoutResponseOrientationHorizontal):
		return LayoutResponseOrientationHorizontal
	case string(LayoutResponseOrientationVertical):
		return LayoutResponseOrientationVertical
	default:
		return def
	}
}

func clampFloat[T ~float64](value, min, max, fallback T) T {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
