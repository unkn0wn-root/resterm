package ui

type rawViewMode int

const (
	rawViewText rawViewMode = iota
	rawViewHex
	rawViewBase64
)

func (m rawViewMode) label() string {
	switch m {
	case rawViewHex:
		return "hex"
	case rawViewBase64:
		return "base64"
	default:
		return "text"
	}
}
