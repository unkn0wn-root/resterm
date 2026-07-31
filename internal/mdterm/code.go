package mdterm

import "strings"

var httpMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
	"HEAD": {}, "OPTIONS": {}, "CONNECT": {}, "TRACE": {},
}

// codeLine highlights one fenced code line. Only http fences carry token
// rules: faint comments with an accented @directive, a bold accent method,
// and template interpolations in the inline-code color. Other languages
// render as written.
func (st styler) codeLine(t, lang string) string {
	if lang != "http" {
		return t
	}
	if strings.HasPrefix(t, "###") {
		return st.span(t, aFaint)
	}
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
		return st.httpComment(t)
	}
	if method, rest, ok := strings.Cut(t, " "); ok {
		if _, known := httpMethods[method]; known {
			return st.span(method, aBold|aAccent) + " " + st.templates(rest)
		}
	}
	return st.templates(t)
}

func (st styler) httpComment(t string) string {
	i := strings.IndexByte(t, '@')
	if i < 0 {
		return st.span(t, aFaint)
	}
	j := i + 1
	for j < len(t) && isDirectiveByte(t[j]) {
		j++
	}
	return st.span(t[:i], aFaint) + st.span(t[i:j], aAccent) + st.span(t[j:], aFaint)
}

func (st styler) templates(t string) string {
	var b strings.Builder
	for {
		i := strings.Index(t, "{{")
		j := strings.Index(t, "}}")
		if i < 0 || j < i {
			break
		}
		b.WriteString(t[:i])
		b.WriteString(st.span(t[i:j+2], aCode))
		t = t[j+2:]
	}
	b.WriteString(t)
	return b.String()
}

func isDirectiveByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}
