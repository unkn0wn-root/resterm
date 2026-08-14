package prompt

import (
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/util"
)

type PathSpec struct {
	Root        string
	Accept      func(path string) bool
	FileSummary string
	Confine     bool
	CommaList   bool
	AcceptDirs  bool
	ExpandHome  bool
	Quote       func(string) string
}

type PathRequest struct {
	Value string
	Edit  Edit
	Spec  PathSpec
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
}

func newPathQuery(r PathRequest) (pathQuery, bool) {
	value := r.Value
	committed := ""
	if r.Spec.CommaList {
		if comma := strings.LastIndexByte(value, ','); comma >= 0 {
			committed, value = value[:comma+1], value[comma+1:]
		}
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
	}, true
}

func (q pathQuery) items(entries []DirEntry) []Item {
	out := make([]Item, 0, len(entries))
	hidden := strings.HasPrefix(q.prefix, ".")
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name, q.prefix) {
			continue
		}
		if !hidden && strings.HasPrefix(entry.Name, ".") {
			continue
		}
		if q.spec.CommaList && strings.ContainsRune(entry.Name, ',') {
			continue
		}
		if !entry.Dir && !q.accepts(filepath.Join(q.dir, entry.Name)) {
			continue
		}

		segment := q.typed + entry.Name
		summary := q.spec.FileSummary
		if entry.Dir {
			segment += string(filepath.Separator)
			summary = "directory"
		}
		out = append(out, Item{
			Label:    segment,
			Summary:  summary,
			Edit:     q.replace(q.committed + segment),
			Continue: entry.Dir && !q.spec.AcceptDirs,
		})
	}
	return out
}

func (q pathQuery) accepts(path string) bool {
	return q.spec.Accept != nil && q.spec.Accept(path)
}

func (q pathQuery) replace(value string) Edit {
	if q.spec.Quote != nil {
		value = q.spec.Quote(value)
	}
	return Edit{Start: q.edit.Start, End: q.edit.End, Text: value}
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
