package restfile

import "strings"

// IsMultipartMime reports whether ct names a multipart media type. ct may be a
// raw header value with parameters, e.g. "multipart/form-data; boundary=x".
func IsMultipartMime(ct string) bool {
	ct = strings.TrimSpace(ct)
	const prefix = "multipart/"
	return len(ct) >= len(prefix) && strings.EqualFold(ct[:len(prefix)], prefix)
}
