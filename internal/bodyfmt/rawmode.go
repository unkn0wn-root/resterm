package bodyfmt

import (
	"slices"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

const (
	RawHeavyLimit      = 128 * 1024
	RawBase64LineWidth = 76
)

type RawMode int

const (
	RawText RawMode = iota
	RawHex
	RawBase64
	RawSummary
)

func (m RawMode) String() string {
	switch m {
	case RawHex:
		return "hex"
	case RawBase64:
		return "base64"
	case RawSummary:
		return "summary"
	default:
		return "text"
	}
}

// Payload is a body's analysis plus the size of the bytes behind it. Size may
// differ from Meta.Size when the analysis came from a substituted view body;
// leave it zero to fall back to the analyzed size.
type Payload struct {
	Meta binaryview.Meta
	Size int
}

func RawHeavy(size int) bool {
	return size > RawHeavyLimit
}

// Modes lists the raw views that make sense for the payload, best first.
func (p Payload) Modes() []RawMode {
	if !p.binary() {
		return []RawMode{RawText, RawHex, RawBase64}
	}
	if p.heavy() {
		return []RawMode{RawSummary, RawHex, RawBase64}
	}
	return []RawMode{RawHex, RawBase64}
}

func (p Payload) ClampMode(mode RawMode) RawMode {
	modes := p.Modes()
	if slices.Contains(modes, mode) {
		return mode
	}
	return modes[0]
}

func (p Payload) ModeLabels() []string {
	modes := p.Modes()
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.String())
	}
	return labels
}

// defaultMode picks the raw view to open on. hasHex reports whether the hex
// dump was materialised: without it we drop to the summary for heavy binaries
// and to the text view for everything else.
func (p Payload) defaultMode(hasHex bool) RawMode {
	mode := RawText
	if p.Meta.Kind == binaryview.KindBinary {
		mode = RawHex
		if p.heavyBinary() {
			mode = RawSummary
		}
	}

	mode = p.ClampMode(mode)
	if mode == RawHex && !hasHex {
		if p.heavyBinary() {
			return RawSummary
		}
		return RawText
	}
	return mode
}

func (p Payload) bytes() int {
	if p.Size > 0 {
		return p.Size
	}
	return p.Meta.Size
}

func (p Payload) binary() bool {
	return p.Meta.Kind == binaryview.KindBinary && !p.Meta.Printable
}

// readable reports whether the payload can be shown as text at all.
func (p Payload) readable() bool {
	return !p.binary()
}

func (p Payload) heavy() bool {
	return RawHeavy(p.bytes())
}

func (p Payload) heavyBinary() bool {
	return p.binary() && p.heavy()
}
