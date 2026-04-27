package bodyref

import (
	"strings"
	"unicode"
	"unicode/utf8"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Location int

const (
	// Line parses a standalone body reference line, e.g. "< ./payload.json".
	Line Location = iota
	// Inline parses a body reference embedded in an at-prefixed line, e.g. "@query < ./query.graphql".
	Inline
)

type Compatibility int

const (
	// ExplicitOnly accepts only the documented "< path" form.
	ExplicitOnly Compatibility = iota
	// AllowNoSpace also accepts the "<path" form for existing request files.
	AllowNoSpace
)

// Parse returns the body file path from Resterm's "< path" body reference syntax.
// Location controls where to look for "<". Compatibility controls whether to
// allow the older no-space form. ExplicitOnly avoids treating XML lines like
// "<soap:Envelope" as file references.
func Parse(line string, loc Location, compat Compatibility) (string, bool) {
	s := str.Trim(line)
	t, ok := tail(s, loc)
	if !ok {
		return "", false
	}
	return parsePath(t, compat)
}

func tail(s string, loc Location) (string, bool) {
	switch loc {
	case Line:
		if !strings.HasPrefix(s, "<") {
			return "", false
		}
		return s[1:], true
	case Inline:
		if !strings.HasPrefix(s, "@") {
			return "", false
		}
		_, after, ok := strings.Cut(s, "<")
		if !ok {
			return "", false
		}
		return after, true
	default:
		return "", false
	}
}

func parsePath(s string, compat Compatibility) (string, bool) {
	explicit := hasLeadingSpace(s)
	if !explicit && compat != AllowNoSpace {
		return "", false
	}

	p := str.Trim(s)
	if p == "" {
		return "", false
	}
	if !explicit && strings.Contains(p, ">") {
		return "", false
	}
	return p, true
}

func hasLeadingSpace(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}
