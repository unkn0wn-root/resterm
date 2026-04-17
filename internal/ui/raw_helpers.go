package ui

import (
	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
)

const (
	rawHeavyLimit      = bodyfmt.RawHeavyLimit
	rawBase64LineWidth = bodyfmt.RawBase64LineWidth
)

func rawHeavy(sz int) bool {
	return bodyfmt.RawHeavy(sz)
}

func rawSum(meta binaryview.Meta, sz int) string {
	return bodyfmt.RawSummaryText(meta, sz)
}
