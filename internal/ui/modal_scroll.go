package ui

import "github.com/charmbracelet/bubbles/viewport"

// scrollViewportKey applies the shared modal scroll bindings and reports
// whether the key was one of them.
func scrollViewportKey(vp *viewport.Model, key string) bool {
	if vp == nil {
		return false
	}
	switch key {
	case "down", "j":
		vp.ScrollDown(1)
	case "up", "k":
		vp.ScrollUp(1)
	case "pgdown", "ctrl+f":
		vp.ScrollDown(max(vp.Height, 1))
	case "pgup", "ctrl+b", "ctrl+u":
		vp.ScrollUp(max(vp.Height, 1))
	case "home", "g":
		vp.GotoTop()
	case "end", "shift+g", "G":
		vp.GotoBottom()
	default:
		return false
	}
	return true
}
