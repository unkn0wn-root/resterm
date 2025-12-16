package binaryview

import (
	"encoding/base64"
	"encoding/hex"
	"strings"
)

func HexPreview(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	return strings.ToLower(hex.EncodeToString(body))
}

// Base64Lines wraps base64 output at a given width.
// Long base64 strings are unreadable and break terminal rendering,
// so we chunk them into lines like PEM files do.
func Base64Lines(body []byte, width int) string {
	if len(body) == 0 {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(body)
	if width <= 0 {
		return encoded
	}

	var b strings.Builder
	for i := 0; i < len(encoded); i += width {
		end := min(i+width, len(encoded))
		b.WriteString(encoded[i:end])
		if end < len(encoded) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func HexDump(body []byte, bytesPerLine int) string {
	if bytesPerLine <= 0 {
		bytesPerLine = 16
	}

	var b strings.Builder
	for offset := 0; offset < len(body); offset += bytesPerLine {
		end := min(offset+bytesPerLine, len(body))
		segment := body[offset:end]
		b.WriteString(formatLine(offset, segment, bytesPerLine))
		if end < len(body) {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatLine(offset int, segment []byte, width int) string {
	var b strings.Builder
	b.WriteString(formatOffset(offset))
	b.WriteString("  ")
	b.WriteString(formatHexCells(segment, width))
	b.WriteString("  ")
	b.WriteString(asciiHint(segment))
	return b.String()
}

func formatOffset(offset int) string {
	return strings.ToUpper(hex32(uint32(offset)))
}

// hex32 converts a uint32 to 8-char hex without fmt.Sprintf.
// We do this manually because hex dumps can have thousands of lines
// and fmt.Sprintf is surprisingly slow when called in a tight loop.
func hex32(v uint32) string {
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = hexDigitsUpper[v&0xF]
		v >>= 4
	}
	return string(out)
}

// formatHexCells renders bytes as "0a 1b 2c" with padding for short lines.
// We pre-allocate the exact buffer size and fill it directly instead of
// using fmt or strings.Builder - again, performance matters for big dumps.
// Short lines get space-padded so the ASCII column stays aligned.
func formatHexCells(segment []byte, width int) string {
	cells := make([]byte, width*3-1)
	for cell := range width {
		cellIdx := cell * 3
		if cell > 0 {
			cells[cellIdx-1] = ' '
		}
		if cell >= len(segment) {
			cells[cellIdx] = ' '
			cells[cellIdx+1] = ' '
			continue
		}
		val := segment[cell]
		cells[cellIdx] = hexDigitsLower[val>>4]
		cells[cellIdx+1] = hexDigitsLower[val&0x0F]
	}
	return string(cells)
}

// asciiHint shows printable ASCII chars, dots for everything else.
// This is the rightmost column in hex dumps - helps you spot strings
// buried in binary data (like "PNG" or "JFIF" magic bytes).
func asciiHint(segment []byte) string {
	out := make([]byte, len(segment))
	for i, b := range segment {
		if b < asciiPrintableMin || b > asciiPrintableMax {
			out[i] = '.'
		} else {
			out[i] = b
		}
	}
	return string(out)
}
