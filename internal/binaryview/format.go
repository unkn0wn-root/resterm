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
		end := i + width
		if end > len(encoded) {
			end = len(encoded)
		}
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
		end := offset + bytesPerLine
		if end > len(body) {
			end = len(body)
		}

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

func hex32(v uint32) string {
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = hexDigitsUpper[v&0xF]
		v >>= 4
	}
	return string(out)
}

func formatHexCells(segment []byte, width int) string {
	cells := make([]byte, width*3-1)
	for cell := 0; cell < width; cell++ {
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
