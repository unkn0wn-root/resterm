package prompt

import (
	"cmp"
	"os"
	"slices"
	"strings"
)

type DirEntry struct {
	Name string
	Dir  bool
}

type DirLoad struct {
	Dir string
	Gen uint64
}

func (l DirLoad) Pending() bool { return l.Dir != "" }

type DirRead struct {
	DirLoad
	Entries []DirEntry
	Err     error
}

func ReadDir(dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dir)
	out := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, DirEntry{Name: entry.Name(), Dir: entry.IsDir()})
	}

	slices.SortFunc(out, func(a, b DirEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Name, b.Name),
		)
	})
	return out, err
}
