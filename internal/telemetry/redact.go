package telemetry

import (
	"net/url"
	"strings"
)

const redactedValue = "REDACTED"

type safeURL struct {
	full   string
	target string
}

func sanitizeURL(u *url.URL) safeURL {
	if u == nil {
		return safeURL{}
	}

	clean := *u
	clean.User = nil
	clean.RawQuery = redactQuery(u.RawQuery)
	clean.Fragment = ""
	clean.RawFragment = ""

	return safeURL{full: clean.String(), target: clean.RequestURI()}
}

func redactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "&")
	for i, part := range parts {
		if key, _, ok := strings.Cut(part, "="); ok {
			parts[i] = key + "=" + redactedValue
		}
	}
	return strings.Join(parts, "&")
}

var scrubSchemes = []string{"https", "http", "wss", "ws"}

func scrubText(text string) string {
	var out strings.Builder
	rest := text
	for {
		at := indexURL(rest)
		if at < 0 {
			break
		}
		out.WriteString(rest[:at])
		raw, tail := splitURL(rest[at:])
		out.WriteString(sanitizeRawURL(raw))
		rest = tail
	}
	if out.Len() == 0 {
		return text
	}
	out.WriteString(rest)
	return out.String()
}

func indexURL(text string) int {
	for at := 0; at < len(text); at++ {
		i := strings.IndexByte(text[at:], ':')
		if i < 0 {
			return -1
		}
		at += i
		if !strings.HasPrefix(text[at:], "://") {
			continue
		}
		if start, ok := schemeStart(text[:at]); ok {
			return start
		}
	}
	return -1
}

func schemeStart(text string) (int, bool) {
	for _, scheme := range scrubSchemes {
		start := len(text) - len(scheme)
		if start < 0 || !strings.EqualFold(text[start:], scheme) {
			continue
		}
		if start > 0 && inScheme(text[start-1]) {
			return 0, false
		}
		return start, true
	}
	return 0, false
}

func inScheme(ch byte) bool {
	switch {
	case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
		return true
	default:
		return ch == '+' || ch == '-' || ch == '.'
	}
}

func splitURL(text string) (string, string) {
	end := len(text)
	for i := range len(text) {
		if endsURL(text[i]) {
			end = i
			break
		}
	}
	for end > 0 && strings.IndexByte(".,;:!?", text[end-1]) >= 0 {
		end--
	}
	return text[:end], text[end:]
}

func endsURL(ch byte) bool {
	return ch <= ' ' || ch == 0x7f || strings.IndexByte(`"'<>\^`+"`"+`{}|`, ch) >= 0
}

func sanitizeRawURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return sanitizeURL(u).full
}
