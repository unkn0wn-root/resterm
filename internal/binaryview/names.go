package binaryview

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

// FilenameHint tries to figure out a good filename for saving a response.
// We check multiple sources in order:
// 1. Content-Disposition header (server's explicit suggestion)
// 2. URL path (often contains the actual filename)
// 3. MIME type (to at least get the right extension)
// 4. Fall back to "response.bin" if all else fails
func FilenameHint(disposition, rawURL, mimeType string) string {
	name := filenameFromDisposition(disposition)
	if name == "" {
		name = filenameFromURL(rawURL)
	}

	ext := extensionForMIME(mimeType)
	if name == "" {
		name = "response"
	}
	if path.Ext(name) == "" && ext != "" {
		name += ext
	}
	if path.Ext(name) == "" {
		name += ".bin"
	}
	return sanitizeFilename(name)
}

// filenameFromDisposition extracts filename from Content-Disposition header.
// Servers can use either "filename" (ASCII only) or "filename*" (RFC 5987,
// supports unicode). We prefer filename* when present since it handles
// international characters properly.
func filenameFromDisposition(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	if v := params["filename*"]; v != "" {
		if decoded := decodeRFC5987(v); decoded != "" {
			return decoded
		}
	}
	if v := params["filename"]; v != "" {
		return sanitizeFilename(v)
	}
	return ""
}

// decodeRFC5987 handles the extended filename encoding from RFC 5987.
// Format is: charset'language'percent-encoded-value
// Examplee: utf-8”%E4%B8%AD%E6%96%87.pdf -> 中文.pdf (got this from google translate :))
// This exists because HTTP headers are ASCII-only, so unicode filenames
// need special encoding. Without this, downloading "données.xlsx" from
// a french server would give us garbage.
func decodeRFC5987(value string) string {
	parts := strings.SplitN(value, "''", 2)
	raw := value
	if len(parts) == 2 {
		raw = parts[1]
	}

	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return ""
	}
	return sanitizeFilename(decoded)
}

func filenameFromURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	base := path.Base(parsed.Path)
	return sanitizeFilename(base)
}

func extensionForMIME(mimeType string) string {
	if strings.TrimSpace(mimeType) == "" {
		return ""
	}

	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ""
	}
	for _, ext := range exts {
		if strings.TrimSpace(ext) != "" {
			return ext
		}
	}
	return ""
}

func sanitizeFilename(name string) string {
	clean := strings.TrimSpace(name)
	clean = strings.ReplaceAll(clean, "\\", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = path.Base(clean)
	clean = strings.Trim(clean, ".")
	if clean == "" {
		return ""
	}
	return clean
}
