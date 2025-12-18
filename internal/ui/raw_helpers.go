package ui

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
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
