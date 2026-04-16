package binaryview

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

// FilenameHint tries to figure out a good filename for saving a response.
// Order:
// 1) Content-Disposition header
// 2) URL path
// 3) MIME type extension
// 4) Fallback to "response.bin"
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

// filenameFromDisposition extracts filename from Content-Disposition.
//
// In Go (after reading source code - yeah, sometimes you must),
// mime.ParseMediaType already decodes RFC 5987/RFC 2231
// parameters (like filename*) and stores the decoded result in params["filename"].
// So we should read params["filename"], not params["filename*"].
func filenameFromDisposition(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	// this may already be the decoded filename* value when present.
	if v := strings.TrimSpace(params["filename"]); v != "" {
		return sanitizeFilename(v)
	}
	return ""
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

	// avoid returning "/" or "." which sanitize into "_" or "" and look broken.
	if base == "" || base == "/" || base == "." {
		return ""
	}

	name := sanitizeFilename(base)

	// special-case: "/" sanitizes to "_" in our sanitizer - treat that as empty.
	if name == "" || name == "_" {
		return ""
	}

	return name
}

func extensionForMIME(mimeType string) string {
	mt := strings.TrimSpace(mimeType)
	if mt == "" {
		return ""
	}

	// "application/json; charset=utf-8"
	if mediaType, _, err := mime.ParseMediaType(mt); err == nil && mediaType != "" {
		mt = strings.ToLower(mediaType)
	}

	exts, err := mime.ExtensionsByType(mt)
	if err != nil || len(exts) == 0 {
		return ""
	}
	if ext := preferredExtensionForMIME(mt, exts); ext != "" {
		return ext
	}
	for _, ext := range exts {
		if e := strings.ToLower(strings.TrimSpace(ext)); e != "" {
			return e
		}
	}
	return ""
}

func preferredExtensionForMIME(mimeType string, exts []string) string {
	preferredByType := map[string][]string{
		"application/json": {".json"},
		"application/pdf":  {".pdf"},
		"application/xml":  {".xml"},
		"image/jpeg":       {".jpg", ".jpeg"},
		"image/png":        {".png"},
		"image/svg+xml":    {".svg"},
		"text/html":        {".html", ".htm"},
		"text/plain":       {".txt"},
		"text/xml":         {".xml"},
	}

	normalized := make(map[string]string, len(exts))
	for _, ext := range exts {
		e := strings.ToLower(strings.TrimSpace(ext))
		if e != "" {
			normalized[e] = e
		}
	}

	for _, candidate := range preferredByType[mimeType] {
		if ext := normalized[candidate]; ext != "" {
			return ext
		}
	}

	slash := strings.IndexByte(mimeType, '/')
	if slash < 0 || slash == len(mimeType)-1 {
		return ""
	}

	subtype := mimeType[slash+1:]
	if plus := strings.IndexByte(subtype, '+'); plus > 0 {
		if ext := normalized["."+subtype[:plus]]; ext != "" {
			return ext
		}
	}
	if ext := normalized["."+subtype]; ext != "" {
		return ext
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
