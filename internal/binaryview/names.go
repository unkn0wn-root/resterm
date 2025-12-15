package binaryview

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

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

func decodeRFC5987(value string) string {
	// ex: utf-8''filename.jpg
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
