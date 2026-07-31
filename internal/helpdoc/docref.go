package helpdoc

import (
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/mod/semver"
)

const (
	docsHost   = "github.com"
	docsRepo   = "unkn0wn-root/resterm"
	manualPath = "docs/resterm.md"
)

var gitDescribePattern = regexp.MustCompile(`-\d+-g[0-9a-f]+(?:-dirty)?$`)

type DocRef struct {
	Path    string
	Heading string
}

func Manual() DocRef {
	return DocRef{Path: manualPath}
}

func manual(heading string) DocRef {
	return DocRef{Path: manualPath, Heading: heading}
}

// DocsRef maps a build version to the git ref documentation links should use.
// Release tags link to their tag. Dev, snapshot, and dirty builds track main.
func DocsRef(version string) string {
	version = strings.TrimSpace(version)
	snapshot := gitDescribePattern.MatchString(version)
	dirty := strings.HasSuffix(version, "-dirty")
	if snapshot || dirty || !semver.IsValid(version) {
		return "main"
	}
	return version
}

func (d DocRef) URL(ref string) string {
	u := url.URL{
		Scheme:   "https",
		Host:     docsHost,
		Path:     path.Join(docsRepo, "blob", ref, d.Path),
		Fragment: d.anchor(),
	}
	return u.String()
}

func (d DocRef) anchor() string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(d.Heading)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
