package theme

import (
	"github.com/charmbracelet/lipgloss"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func ResolveDefinition(c Catalog, key string, fb Theme) Definition {
	if def, ok := c.Get(normalizeKey(key)); ok {
		return def
	}
	def := DefaultDefinition()
	def.Theme = fb
	key = normalizeKey(key)
	if key == "" {
		return def
	}
	def.Key = key
	def.DisplayName = humaniseSlug(key)
	def.Metadata.Name = def.DisplayName
	return def
}

func ActiveTextStyle(th Theme) lipgloss.Style {
	if th.PaneActiveForeground != "" {
		return lipgloss.NewStyle().Foreground(th.PaneActiveForeground)
	}
	if fg := th.ResponseContent.GetForeground(); ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))
}

func ForegroundStyle(base lipgloss.Style, fb lipgloss.Color) lipgloss.Style {
	st := lipgloss.NewStyle()
	if fg := base.GetForeground(); ColorDefined(fg) {
		return st.Foreground(fg)
	}
	return st.Foreground(fb)
}

func ColorDefined(c lipgloss.TerminalColor) bool {
	if c == nil {
		return false
	}
	if _, ok := c.(lipgloss.NoColor); ok {
		return false
	}
	if v, ok := c.(lipgloss.Color); ok && v == "" {
		return false
	}
	return true
}

func normalizeKey(key string) string {
	return str.LowerTrim(key)
}
