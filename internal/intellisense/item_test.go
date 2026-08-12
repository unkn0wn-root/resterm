package intellisense

import (
	"slices"
	"testing"
)

// catalogItems is every static suggestion the engine can offer, so a new entry
// is checked without being listed here.
func catalogItems() []Item {
	items := slices.Clone(directives)
	items = append(items, methods...)
	items = append(items, schemes...)
	items = append(items, headerNameItems...)
	items = append(items, builtinVars...)
	for _, values := range headerValues {
		items = append(items, values...)
	}
	for _, args := range directiveArgs {
		items = append(items, args...)
	}
	return items
}

func TestCatalogPlaceholdersAreInsertedText(t *testing.T) {
	for _, it := range catalogItems() {
		if it.Placeholder == "" {
			continue
		}
		insert := it.InsertText()
		start, end, ok := it.PlaceholderRange()
		if !ok {
			t.Errorf("%s: placeholder %q is not part of %q", it.Label, it.Placeholder, insert)
			continue
		}
		if got := string([]rune(insert)[start:end]); got != it.Placeholder {
			t.Errorf("%s: range covers %q, want %q", it.Label, got, it.Placeholder)
		}
	}
}

func TestPlaceholderRange(t *testing.T) {
	cases := []struct {
		name  string
		item  Item
		start int
		end   int
		ok    bool
	}{
		{
			name:  "value at the end",
			item:  Item{Label: "timeout=", Insert: "timeout=5s", Placeholder: "5s"},
			start: 8,
			end:   10,
			ok:    true,
		},
		{
			name:  "closing paren stays out",
			item:  Item{Label: "latency=", Insert: "latency=random(1s,2s)", Placeholder: "1s,2s"},
			start: 15,
			end:   20,
			ok:    true,
		},
		{
			name:  "repeated value takes the last",
			item:  Item{Label: "scheme=", Insert: "scheme=Bearer Bearer", Placeholder: "Bearer"},
			start: 14,
			end:   20,
			ok:    true,
		},
		{
			name:  "offsets count runes",
			item:  Item{Label: "name=", Insert: "name=Ünïcode", Placeholder: "code"},
			start: 8,
			end:   12,
			ok:    true,
		},
		{
			name: "no placeholder",
			item: Item{Label: "none", Insert: "none"},
		},
		{
			name: "placeholder outside the insert",
			item: Item{Label: "port=", Insert: "port=22", Placeholder: "8080"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end, ok := tc.item.PlaceholderRange()
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if start != tc.start || end != tc.end {
				t.Fatalf("range = [%d,%d), want [%d,%d)", start, end, tc.start, tc.end)
			}
		})
	}
}

func TestInsertTextFallsBackToLabel(t *testing.T) {
	if got := (Item{Label: "GET"}).InsertText(); got != "GET" {
		t.Fatalf("InsertText = %q, want the label", got)
	}
	if got := (Item{Label: "Accept", Insert: "Accept:"}).InsertText(); got != "Accept:" {
		t.Fatalf("InsertText = %q, want the insert", got)
	}
}
