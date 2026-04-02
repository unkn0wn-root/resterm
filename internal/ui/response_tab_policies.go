package ui

// Keep these allowlists explicit so new tabs are opt-in for overlays/async wrap.
const responseWrapAsyncLimit = 32 * 1024

func tabAllowsOverlay(tab responseTab) bool {
	switch tab {
	case responseTabPretty,
		responseTabRaw,
		responseTabHeaders,
		responseTabExplain,
		responseTabStream,
		responseTabStats,
		responseTabTimeline,
		responseTabCompare,
		responseTabDiff:
		return true
	default:
		return false
	}
}

func tabAllowsAsyncWrap(tab responseTab) bool {
	switch tab {
	case responseTabPretty,
		responseTabRaw,
		responseTabExplain,
		responseTabStats,
		responseTabTimeline,
		responseTabCompare,
		responseTabDiff:
		return true
	default:
		return false
	}
}
