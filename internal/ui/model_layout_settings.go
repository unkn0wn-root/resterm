package ui

import "github.com/unkn0wn-root/resterm/internal/config"

func (m *Model) applyLayoutSettingsFromConfig(layout config.LayoutSettings) {
	normalised := config.NormaliseLayoutSettings(layout)
	m.sidebarWidth = normalised.SidebarWidth
	m.editorSplit = normalised.EditorSplit
	m.responseSplitRatio = normalised.ResponseSplitRatio
	m.mainSplitOrientation = mainSplitOrientationFor(normalised.MainSplit)
	m.responseSplitOrientation = responseSplitOrientationFor(normalised.ResponseOrientation)
	if normalised.ResponseSplit {
		m.responseSplit = true
		m.setLivePane(m.responsePaneFocus)
	}
}

func (m *Model) currentLayoutSettings() config.LayoutSettings {
	layout := config.LayoutSettings{
		SidebarWidth:        m.sidebarWidth,
		EditorSplit:         m.editorSplit,
		MainSplit:           config.LayoutMainSplitVertical,
		ResponseSplit:       m.responseSplit,
		ResponseSplitRatio:  m.responseSplitRatio,
		ResponseOrientation: config.LayoutResponseOrientationVertical,
	}
	layout.MainSplit = mainSplitToken(m.mainSplitOrientation)
	layout.ResponseOrientation = responseSplitToken(m.responseSplitOrientation)
	return config.NormaliseLayoutSettings(layout)
}

func mainSplitOrientationFor(token config.LayoutMainSplit) mainSplitOrientation {
	switch token {
	case config.LayoutMainSplitHorizontal:
		return mainSplitHorizontal
	default:
		return mainSplitVertical
	}
}

func responseSplitOrientationFor(token config.LayoutResponseOrientation) responseSplitOrientation {
	switch token {
	case config.LayoutResponseOrientationHorizontal:
		return responseSplitHorizontal
	default:
		return responseSplitVertical
	}
}

func mainSplitToken(orientation mainSplitOrientation) config.LayoutMainSplit {
	switch orientation {
	case mainSplitHorizontal:
		return config.LayoutMainSplitHorizontal
	default:
		return config.LayoutMainSplitVertical
	}
}

func responseSplitToken(orientation responseSplitOrientation) config.LayoutResponseOrientation {
	switch orientation {
	case responseSplitHorizontal:
		return config.LayoutResponseOrientationHorizontal
	default:
		return config.LayoutResponseOrientationVertical
	}
}
