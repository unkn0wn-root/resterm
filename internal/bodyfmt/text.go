package bodyfmt

import (
	"regexp"
	"strings"
)

// ANSISequenceRegex matches complete CSI and OSC sequences only. Truncated or
// unrecognised escapes are left alone so that stripping never eats real text.
var ANSISequenceRegex = regexp.MustCompile(
	"\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)",
)

func StripANSI(s string) string {
	return ANSISequenceRegex.ReplaceAllString(s, "")
}

func IsEmpty(body string) bool {
	return strings.TrimSpace(StripANSI(body)) == ""
}

func TrimBody(body string) string {
	return strings.TrimRight(body, "\n")
}

func TrimSection(section string) string {
	return strings.Trim(section, "\r\n")
}

// JoinSections stacks non-empty sections with a blank line between them.
func JoinSections(sections ...string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		trimmed := TrimSection(section)
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}
