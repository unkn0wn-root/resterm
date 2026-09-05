package prompt

import (
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type PathSpec struct {
	Root        string
	Files       files.PathFilter
	FileSummary string
	Confine     bool
	Separators  SeparatorPolicy
	AcceptDirs  bool
	ExpandHome  bool
	Quote       bool
}

// SeparatorPolicy defines which characters separate paths in a list.
type SeparatorPolicy uint8

const (
	SeparatorNone SeparatorPolicy = iota
	SeparatorComma
	SeparatorPathList
)

func (p SeparatorPolicy) separates(r rune) bool {
	switch p {
	case SeparatorComma:
		return r == ','
	case SeparatorPathList:
		return strings.ContainsRune(",; \t\n", r)
	default:
		return false
	}
}

type PathRequest struct {
	Value  string // decoded value before the caret
	Suffix string // decoded value after the caret
	Edit   Edit
	Spec   PathSpec
}

type PathProvider interface {
	PathAt(input string, cursor int) (PathRequest, bool)
}

type pathQuery struct {
	spec      PathSpec
	edit      Edit
	dir       string
	typed     string
	prefix    string
	committed string
	suffix    string
}

type pathCandidate struct {
	name    string
	summary string
	dir     bool
	hidden  bool
}

func newPathQuery(r PathRequest) (pathQuery, bool) {
	value := r.Value
	committed := ""
	if at := strings.LastIndexFunc(value, r.Spec.Separators.separates); at >= 0 {
		committed, value = value[:at+1], value[at+1:]
	}
	// Preserve later paths when replacing the current entry.
	suffix := ""
	if at := strings.IndexFunc(r.Suffix, r.Spec.Separators.separates); at >= 0 {
		suffix = r.Suffix[at:]
	}

	typed, prefix := filepath.Split(value)
	resolved := value
	if r.Spec.ExpandHome && strings.HasPrefix(value, "~") {
		resolved = util.ExpandHome(value)
		// Treat a bare ~ as a directory, not a name prefix.
		if value == "~" {
			resolved += string(filepath.Separator)
			typed = "~" + string(filepath.Separator)
			prefix = ""
		}
	}
	if r.Spec.Confine && (filepath.IsAbs(resolved) || !filepath.IsLocal(filepath.Clean(resolved))) {
		return pathQuery{}, false
	}

	dir, _ := filepath.Split(resolved)
	switch {
	case dir == "":
		dir = r.Spec.Root
	case !filepath.IsAbs(dir):
		dir = filepath.Join(r.Spec.Root, dir)
	}
	dir = filepath.Clean(dir)
	if r.Spec.Confine && !within(r.Spec.Root, dir) {
		return pathQuery{}, false
	}

	return pathQuery{
		spec:      r.Spec,
		edit:      r.Edit,
		dir:       dir,
		typed:     typed,
		prefix:    prefix,
		committed: committed,
		suffix:    suffix,
	}, true
}

func (q pathQuery) classify(entries []DirEntry) []pathCandidate {
	out := make([]pathCandidate, 0, len(entries))
	for _, entry := range entries {
		if strings.IndexFunc(entry.Name, q.spec.Separators.separates) >= 0 {
			continue
		}
		if !entry.Dir && !q.accepts(filepath.Join(q.dir, entry.Name)) {
			continue
		}

		name := entry.Name
		summary := q.spec.FileSummary
		if entry.Dir {
			name += string(filepath.Separator)
			summary = "directory"
		}
		out = append(out, pathCandidate{
			name:    name,
			summary: summary,
			dir:     entry.Dir,
			hidden:  strings.HasPrefix(entry.Name, "."),
		})
	}
	return out
}

func (q pathQuery) items(candidates []pathCandidate) []Item {
	out := make([]Item, 0, len(candidates))
	for _, candidate := range candidates {
		// Show only the entry; Edit retains the full path.
		out = append(out, Item{
			Label:    candidate.name,
			Summary:  candidate.summary,
			Edit:     q.replace(q.committed + q.typed + candidate.name),
			Continue: candidate.dir && !q.spec.AcceptDirs,
		})
	}
	return out
}

func (q pathQuery) accepts(path string) bool {
	return q.spec.Files.Accept(path)
}

func (q pathQuery) replace(value string) Edit {
	cursor := len([]rune(value))
	value += q.suffix
	if q.spec.Quote {
		value, cursor = quoteAt(value, cursor)
	}
	return Edit{
		Start:      q.edit.Start,
		End:        q.edit.End,
		Text:       value,
		CursorBack: len([]rune(value)) - cursor,
	}
}

func within(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	return err == nil && filepath.IsLocal(rel)
}
