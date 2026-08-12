package intellisense

import (
	"slices"
	"strings"
	"unicode/utf8"
)

type Item struct {
	Label   string
	Aliases []string
	Summary string

	// Insert is written in place of the typed token. Empty inserts Label.
	Insert string

	// Placeholder is the example value inside the inserted text that an editor
	// selects, so the next keystroke types over it. Writing it out instead of a
	// caret offset keeps delimiters around the value, such as a closing paren,
	// outside the selection.
	Placeholder string
}

// InsertText is what an editor writes when the item is accepted.
func (it Item) InsertText() string {
	if it.Insert != "" {
		return it.Insert
	}
	return it.Label
}

// PlaceholderRange locates Placeholder in InsertText as rune offsets. The last
// match wins because a placeholder is the value at the end of an example.
func (it Item) PlaceholderRange() (start, end int, ok bool) {
	if it.Placeholder == "" {
		return 0, 0, false
	}
	text := it.InsertText()
	at := strings.LastIndex(text, it.Placeholder)
	if at < 0 {
		return 0, 0, false
	}
	start = utf8.RuneCountInString(text[:at])
	return start, start + utf8.RuneCountInString(it.Placeholder), true
}

func filter(opts []Item, q string) []Item {
	if len(opts) == 0 {
		return nil
	}
	if q == "" {
		return slices.Clone(opts)
	}
	lq := strings.ToLower(q)
	var out []Item
	for _, opt := range opts {
		if match(opt, lq) {
			out = append(out, opt)
		}
	}
	return out
}

func match(it Item, q string) bool {
	if hasPrefix(it.Label, q) {
		return true
	}
	for _, a := range it.Aliases {
		if hasPrefix(a, q) {
			return true
		}
	}
	return false
}

func hasPrefix(label, q string) bool {
	label = strings.TrimPrefix(label, "@")
	return strings.HasPrefix(strings.ToLower(label), q)
}
