package binaryview

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

var textMIMESubstrings = []string{
	"json",
	"xml",
	"yaml",
	"html",
	"javascript",
	"ecmascript",
	"graphql",
}

type Kind int

const (
	KindUnknown Kind = iota
	KindText
	KindBinary
)

type Meta struct {
	Kind       Kind
	MIME       string
	Charset    string
	Size       int
	Printable  bool
	DecodeErr  string
	PreviewHex string
	PreviewB64 string
}

const (
	printableSampleLimit = 1024
	printableThreshold   = 0.95
	previewByteLimit     = 96
)

func Analyze(body []byte, contentType string) Meta {
	mimeType, charsetLabel := parseContentType(contentType)
	printable := isLikelyPrintable(body)
	kind := decideKind(mimeType, printable)

	preview := trimPreview(body, previewByteLimit)

	return Meta{
		Kind:       kind,
		MIME:       mimeType,
		Charset:    charsetLabel,
		Size:       len(body),
		Printable:  printable,
		PreviewHex: HexPreview(preview),
		PreviewB64: base64.StdEncoding.EncodeToString(preview),
	}
}

func DecodeText(body []byte, charsetLabel string) (string, bool, string) {
	label := strings.TrimSpace(strings.ToLower(charsetLabel))
	if label == "" {
		label = "utf-8"
	}

	reader, err := charset.NewReaderLabel(label, bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Sprintf("charset: %v", err)
	}

	decoded, err := ioReadAll(reader)
	if err != nil {
		return "", false, fmt.Sprintf("decode: %v", err)
	}
	return string(decoded), true, ""
}

func decideKind(mimeType string, printable bool) Kind {
	if mimeType != "" {
		if isTextMIME(mimeType) {
			return KindText
		}
		if printable {
			return KindText
		}
		return KindBinary
	}
	if printable {
		return KindText
	}
	return KindBinary
}

func parseContentType(value string) (mimeType, charsetLabel string) {
	if strings.TrimSpace(value) == "" {
		return "", ""
	}

	mType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value)), ""
	}
	return strings.ToLower(mType), strings.ToLower(params["charset"])
}

func isTextMIME(mimeType string) bool {
	if mimeType == "" {
		return false
	}

	if strings.HasPrefix(mimeType, "text/") {
		return true
	}

	for _, marker := range textMIMESubstrings {
		if strings.Contains(mimeType, marker) {
			return true
		}
	}
	return false
}

func isLikelyPrintable(body []byte) bool {
	if len(body) == 0 {
		return true
	}

	sample := body
	if len(sample) > printableSampleLimit {
		sample = sample[:printableSampleLimit]
	}

	printable := 0
	total := 0
	for len(sample) > 0 {
		r, size := utf8.DecodeRune(sample)
		if r == utf8.RuneError && size == 1 {
			return false
		}
		sample = sample[size:]
		total++
		if isAllowedRune(r) {
			printable++
		}
	}
	if total == 0 {
		return true
	}
	return float64(printable)/float64(total) >= printableThreshold
}

func isAllowedRune(r rune) bool {
	if r == '\n' || r == '\r' || r == '\t' {
		return true
	}
	if r < asciiPrintableMin {
		return false
	}
	return unicode.IsGraphic(r)
}

func trimPreview(body []byte, limit int) []byte {
	if limit <= 0 || len(body) <= limit {
		return body
	}
	return body[:limit]
}

func ioReadAll(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
