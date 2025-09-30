package ui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) activatePrevTab() tea.Cmd {
	prev := m.activeTab
	if m.activeTab == responseTabPretty {
		m.activeTab = responseTabHistory
	} else {
		m.activeTab--
	}
	if m.activeTab == responseTabHistory && prev != responseTabHistory {
		m.historyJumpToLatest = true
	}
	return m.syncResponseContent()
}

func (m *Model) activateNextTab() tea.Cmd {
	prev := m.activeTab
	if m.activeTab == responseTabHistory {
		m.activeTab = responseTabPretty
	} else {
		m.activeTab++
	}
	if m.activeTab == responseTabHistory && prev != responseTabHistory {
		m.historyJumpToLatest = true
	}
	return m.syncResponseContent()
}

func (m *Model) renderViewport(content string) {
	width := m.responseViewport.Width
	if width <= 0 {
		width = 80
	}
	m.responseViewport.SetContent(wrapToWidth(content, width))
}

func (m *Model) syncResponseContent() tea.Cmd {
	if m.activeTab == responseTabHistory {
		return nil
	}

	if m.responseLoading {
		m.responseViewport.SetContent(m.responseLoadingMessage())
		return nil
	}

	width := m.responseViewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	height := m.responseViewport.Height

	source, cache := m.responseSourceForTab(m.activeTab)
	if source == "" {
		centered := centerContent(noResponseMessage, width, height)
		m.responseViewport.SetContent(wrapToWidth(centered, width))
		return nil
	}

	if source == noResponseMessage {
		centered := centerContent(noResponseMessage, width, height)
		wrapped := wrapToWidth(centered, width)
		if cache != nil {
			*cache = cachedWrap{width: width, content: wrapped, valid: true}
		}
		m.responseViewport.SetContent(wrapped)
		return nil
	}

	if cache != nil && cache.valid && cache.width == width {
		m.responseViewport.SetContent(cache.content)
		return nil
	}

	if cache != nil && m.responseRenderToken != "" {
		m.responseViewport.SetContent(responseReflowingMessage)
		return wrapResponseContentCmd(m.responseRenderToken, m.prettyView, m.rawView, m.headersView, width)
	}

	wrapped := wrapToWidth(source, width)
	if cache != nil {
		*cache = cachedWrap{width: width, content: wrapped, valid: true}
	}
	m.responseViewport.SetContent(wrapped)
	return nil
}

func (m *Model) responseSourceForTab(tab responseTab) (string, *cachedWrap) {
	switch tab {
	case responseTabPretty:
		return m.prettyView, &m.prettyWrapCache
	case responseTabRaw:
		return m.rawView, &m.rawWrapCache
	case responseTabHeaders:
		return m.headersView, &m.headersWrapCache
	default:
		return "", nil
	}
}
