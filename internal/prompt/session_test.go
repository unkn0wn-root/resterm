package prompt

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/files"
)

type pathProviderFunc func(input string, cursor int) (PathRequest, bool)

func (f pathProviderFunc) PathAt(input string, cursor int) (PathRequest, bool) {
	return f(input, cursor)
}

func TestPathSessionCachesClassificationAndPrefixes(t *testing.T) {
	root := t.TempDir()
	p := pathProviderFunc(func(input string, cursor int) (PathRequest, bool) {
		return PathRequest{
			Value: input,
			Edit:  Edit{End: cursor},
			Spec: PathSpec{
				Root:        root,
				Files:       files.AnyPathFilter(),
				FileSummary: "file",
			},
		}, true
	})
	entries := []DirEntry{
		{Name: "api", Dir: true},
		{Name: ".env"},
		{Name: "alpha.http"},
		{Name: "alpine.http"},
		{Name: "beta.http"},
	}

	var s PathSession
	_, load, ok := s.Suggest(p, "", 0)
	if !ok || !load.Pending() {
		t.Fatal("initial suggestion did not request its directory")
	}
	items, ok := s.Deliver(DirRead{DirLoad: load, Entries: entries})
	if !ok {
		t.Fatal("directory read was not accepted")
	}
	all := []string{"api" + string(filepath.Separator), "alpha.http", "alpine.http", "beta.http"}
	if got := itemLabels(items); !slices.Equal(got, all) {
		t.Fatalf("initial items = %q, want %q", got, all)
	}
	listing := s.cache[load.Dir]
	if listing == nil || len(listing.prepared) != 1 {
		t.Fatalf("prepared listings = %d, want 1", len(listing.prepared))
	}
	var prepared *pathCandidates
	for _, c := range listing.prepared {
		prepared = c
	}

	tests := []struct {
		prefix string
		want   []string
	}{
		{"a", []string{"api" + string(filepath.Separator), "alpha.http", "alpine.http"}},
		{"al", []string{"alpha.http", "alpine.http"}},
		{"alp", []string{"alpha.http", "alpine.http"}},
		{"al", []string{"alpha.http", "alpine.http"}},
		{".", []string{".env"}},
		{"", all},
	}
	for _, tt := range tests {
		items, load, ok = s.Suggest(p, tt.prefix, len([]rune(tt.prefix)))
		if !ok || load.Pending() {
			t.Fatalf("prefix %q unexpectedly requested a directory", tt.prefix)
		}
		if got := itemLabels(items); !slices.Equal(got, tt.want) {
			t.Fatalf("prefix %q items = %q, want %q", tt.prefix, got, tt.want)
		}
	}
	if len(listing.prepared) != 1 {
		t.Fatalf("typing rebuilt %d prepared listings, want 1", len(listing.prepared))
	}
	for _, c := range listing.prepared {
		if c != prepared {
			t.Fatal("typing replaced the prepared directory classification")
		}
	}
}

func TestPathSessionNarrowsPrefixesCorrectly(t *testing.T) {
	root := t.TempDir()
	p := pathProviderFunc(func(input string, cursor int) (PathRequest, bool) {
		return PathRequest{
			Value: input,
			Edit:  Edit{End: cursor},
			Spec:  PathSpec{Root: root, Files: files.AnyPathFilter(), FileSummary: "file"},
		}, true
	})
	entries := []DirEntry{
		{Name: "界面.http"},
		{Name: "界限.http"},
		{Name: "плюс.http"},
		{Name: ".界面.http"},
		{Name: "plain.http"},
	}

	var s PathSession
	_, load, _ := s.Suggest(p, "", 0)
	if _, ok := s.Deliver(DirRead{DirLoad: load, Entries: entries}); !ok {
		t.Fatal("directory read was not accepted")
	}

	for _, tt := range []struct {
		prefix string
		want   []string
	}{
		{"界", []string{"界面.http", "界限.http"}},
		{"界面", []string{"界面.http"}},
		{"界", []string{"界面.http", "界限.http"}},
		{"п", []string{"плюс.http"}},
		{".", []string{".界面.http"}},
		{".界", []string{".界面.http"}},
		{"", []string{"界面.http", "界限.http", "плюс.http", "plain.http"}},
	} {
		items, load, ok := s.Suggest(p, tt.prefix, len([]rune(tt.prefix)))
		if !ok || load.Pending() {
			t.Fatalf("prefix %q unexpectedly requested a directory", tt.prefix)
		}
		if got := itemLabels(items); !slices.Equal(got, tt.want) {
			t.Fatalf("prefix %q items = %q, want %q", tt.prefix, got, tt.want)
		}
	}
}

func TestPathSessionSeparatesSpecsThatShareADirectory(t *testing.T) {
	root := t.TempDir()
	entries := []DirEntry{{Name: "one two.http"}}
	spec := PathSpec{Root: root, Files: files.AnyPathFilter(), FileSummary: "file"}
	provider := func(s PathSpec) pathProviderFunc {
		return func(input string, cursor int) (PathRequest, bool) {
			return PathRequest{Value: input, Edit: Edit{End: cursor}, Spec: s}, true
		}
	}

	var s PathSession
	_, load, _ := s.Suggest(provider(spec), "", 0)
	if _, ok := s.Deliver(DirRead{DirLoad: load, Entries: entries}); !ok {
		t.Fatal("directory read was not accepted")
	}

	quoted := spec
	quoted.Quote = true
	items, _, _ := s.Suggest(provider(quoted), "", 0)
	if len(items) != 1 || items[0].Edit.Text != `"one two.http"` {
		t.Fatalf("quoting spec reused the unquoted edit: %#v", items)
	}
	plain, _, _ := s.Suggest(provider(spec), "", 0)
	if len(plain) != 1 || plain[0].Edit.Text != "one two.http" {
		t.Fatalf("plain spec reused the quoted edit: %#v", plain)
	}
}

func TestPathSessionKeepsFiltersSeparate(t *testing.T) {
	root := t.TempDir()
	entries := []DirEntry{{Name: "one.http"}, {Name: "two.json"}}
	provider := func(summary string, filter files.PathFilter) pathProviderFunc {
		return func(input string, cursor int) (PathRequest, bool) {
			return PathRequest{
				Value: input,
				Edit:  Edit{End: cursor},
				Spec:  PathSpec{Root: root, Files: filter, FileSummary: summary},
			}, true
		}
	}

	var s PathSession
	requests := provider("request", files.RequestPathFilter())
	_, load, _ := s.Suggest(requests, "", 0)
	items, _ := s.Deliver(DirRead{DirLoad: load, Entries: entries})
	if got := itemLabels(items); !slices.Equal(got, []string{"one.http"}) {
		t.Fatalf("request items = %q", got)
	}

	all := provider("all files", files.AnyPathFilter())
	items, load, _ = s.Suggest(all, "", 0)
	if load.Pending() {
		t.Fatal("changing the filter reread an already loaded directory")
	}
	if got := itemLabels(items); !slices.Equal(got, []string{"one.http", "two.json"}) {
		t.Fatalf("all-file items = %q", got)
	}
}

func BenchmarkPathSessionSuggestions(b *testing.B) {
	for _, n := range []int{100, 1_000, 10_000} {
		b.Run(fmt.Sprintf("entries_%d", n), func(b *testing.B) {
			entries := make([]DirEntry, n)
			for i := range entries {
				entries[i] = DirEntry{Name: fmt.Sprintf("request-%05d.http", i)}
			}
			p := pathProviderFunc(func(input string, cursor int) (PathRequest, bool) {
				return PathRequest{
					Value: input,
					Edit:  Edit{End: cursor},
					Spec: PathSpec{
						Root:        "/workspace",
						Files:       files.AnyPathFilter(),
						FileSummary: "request file",
					},
				}, true
			})

			b.ReportAllocs()
			for b.Loop() {
				var s PathSession
				_, load, _ := s.Suggest(p, "", 0)
				s.Deliver(DirRead{DirLoad: load, Entries: entries})
				for _, prefix := range []string{"r", "re", "req", "request-9"} {
					s.Suggest(p, prefix, len(prefix))
				}
			}
		})
	}
}

func itemLabels(items []Item) []string {
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	return labels
}
