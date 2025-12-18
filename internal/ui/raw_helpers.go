package ui

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

const (
	// rawHeavyLimit marks the payload size above which we defer expensive dumps.
	rawHeavyLimit = 128 * 1024
	// rawBase64LineWidth mirrors PEM-style wrapping to keep long dumps readable.
	rawBase64LineWidth = 76
)

func rawHeavy(sz int) bool {
	return sz > rawHeavyLimit
}

func rawHeavyBin(meta binaryview.Meta, sz int) bool {
	if meta.Kind != binaryview.KindBinary || meta.Printable {
		return false
	}
	if sz <= 0 {
		sz = meta.Size
	}
	return rawHeavy(sz)
}

func rawSum(meta binaryview.Meta, sz int) string {
	if sz <= 0 {
		sz = meta.Size
	}
	szStr := formatByteSize(int64(sz))
	mime := strings.TrimSpace(meta.MIME)
	hdr := fmt.Sprintf("Binary body (%s)", szStr)
	if mime != "" {
		hdr = fmt.Sprintf("Binary body (%s, %s)", szStr, mime)
	}
	return hdr + "\n<raw dump deferred>\nUse the raw view action to load hex/base64."
}
