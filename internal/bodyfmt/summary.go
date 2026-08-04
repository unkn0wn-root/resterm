package bodyfmt

import (
	"fmt"
	"strings"
)

type SummaryKind int

const (
	SummaryTitle SummaryKind = iota
	SummaryField
	SummaryWarn
	SummaryPreview
	SummaryModes
)

// SummaryLine is one row of a binary body summary. Callers that style their
// output switch on Kind and keep their own labels; plain callers use String.
type SummaryLine struct {
	Kind  SummaryKind
	Label string
	Value string
}

func (l SummaryLine) String() string {
	if l.Label == "" {
		return l.Value
	}
	return l.Label + ": " + l.Value
}

func (p Payload) BinarySummary() []SummaryLine {
	lines := []SummaryLine{{
		Kind:  SummaryTitle,
		Value: fmt.Sprintf("Binary body (%s)", FormatByteSize(int64(p.bytes()))),
	}}
	if mime := strings.TrimSpace(p.Meta.MIME); mime != "" {
		lines = append(lines, SummaryLine{Kind: SummaryField, Label: "MIME", Value: mime})
	}
	if warn := strings.TrimSpace(p.Meta.DecodeErr); warn != "" {
		lines = append(
			lines,
			SummaryLine{Kind: SummaryWarn, Label: "Decode warning", Value: warn},
		)
	}
	if hex := p.Meta.PreviewHex; hex != "" {
		lines = append(
			lines,
			SummaryLine{Kind: SummaryPreview, Label: "Preview hex", Value: hex},
		)
	}
	if b64 := p.Meta.PreviewB64; b64 != "" {
		lines = append(
			lines,
			SummaryLine{Kind: SummaryPreview, Label: "Preview base64", Value: b64},
		)
	}
	if modes := p.ModeLabels(); len(modes) > 0 {
		lines = append(lines, SummaryLine{
			Kind:  SummaryModes,
			Label: "Raw view",
			Value: strings.Join(modes, " / "),
		})
	}
	return lines
}

func (p Payload) BinarySummaryText() string {
	lines := p.BinarySummary()
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.String())
	}
	return strings.Join(out, "\n")
}

// RawSummaryText stands in for a raw dump we refused to build because the body
// is too big to render up front.
func (p Payload) RawSummaryText() string {
	size := FormatByteSize(int64(p.bytes()))
	title := fmt.Sprintf("Binary body (%s)", size)
	if mime := strings.TrimSpace(p.Meta.MIME); mime != "" {
		title = fmt.Sprintf("Binary body (%s, %s)", size, mime)
	}
	return title + "\n<raw dump deferred>\nUse the raw view action to load hex/base64."
}
