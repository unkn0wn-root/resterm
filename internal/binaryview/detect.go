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

// These MIME types are technically not "text/*" but everyone treats them as text.
// Without this list, "application/json" would show up as binary gibberish.
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
	// We only scan the first 1KB to decide if something is text or binary.
	// Scanning the whole body would be slow for large files, and the first
	// chunk is usually enough to tell - binary files have junk bytes early on.
	printableSampleLimit = 1024

	// 95% printable chars = text. We allow some slack because "real worldd" text
	// files sometimes have a few stray bytes (BOM, weird line endings, etc).
	printableThreshold = 0.95

	// Preview limit for hex/base64 snippets shown in the UI summary.
	previewByteLimit = 96
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

// decideKind figures out if we should treat response as text or binary.
// Priority: trust MIME type first, fall back to byte analysis if unknown.
// This matters because some servers lie about Content-Type, so we double check
// with actual byte inspection when the MIME says binary but content looks like text.
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

// isLikelyPrintable does a quick UTF-8 validity check on the body.
// If we hit an invalid UTF-8 sequence, it's definitely binary. Otherwise
// we count printable vs non-printable runes and use a threshold.
// This catches edge cases where Content-Type is missing or wrong.
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
		// Invalid UTF-8 byte sequence - almost certainly binary data.
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

// isAllowedRune decides if a character is "printable" for our purposes.
// We allow common whitespace (tabs, newlines) because text files have those.
// Everything else must be a visible graphic character - control codes like
// NUL, BEL, ESC etc. are signs of binary data.
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
