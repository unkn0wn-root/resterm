package ui

type rawViewMode int

const (
	rawViewText rawViewMode = iota
	rawViewHex
	rawViewBase64
	rawViewSummary
)

func (m rawViewMode) label() string {
	switch m {
	case rawViewHex:
		return "hex"
	case rawViewBase64:
		return "base64"
	case rawViewSummary:
		return "summary"
	default:
		return "text"
	}
}
