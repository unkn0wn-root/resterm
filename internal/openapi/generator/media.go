package generator

import (
	"mime"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func baseMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		return strings.ToLower(mediaType)
	}
	mediaType, _, _ = strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func safeContentType(contentType string) (string, bool) {
	contentType = strings.TrimSpace(contentType)
	if contentType != "" && !httpguts.ValidHeaderFieldValue(contentType) {
		return "", false
	}
	return contentType, true
}

func isJSONMedia(contentType string) bool {
	contentType = baseMediaType(contentType)
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}
