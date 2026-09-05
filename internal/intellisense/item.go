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

	// Insert replaces the typed token. If empty, Label is used.
	Insert string

	// CursorBack leaves the caret this many runes before the end of InsertText.
	CursorBack int

	// Placeholder is the inserted example text selected for replacement.
	// Surrounding delimiters, such as parentheses, stay outside the selection.
	Placeholder string

	// Continue opens suggestions at the new caret position after insertion.
	Continue bool

	noTrailingSpace bool
}

// WithoutTrailingSpace returns a copy that adds no space after insertion.
func (it Item) WithoutTrailingSpace() Item {
	it.noTrailingSpace = true
	return it
}

// InsertText returns the text to insert when the item is accepted.
func (it Item) InsertText() string {
	if it.Insert != "" {
		return it.Insert
	}
	return it.Label
}

func cutCall(usage string) (call, argv string) {
	name, rest, ok := strings.Cut(usage, "(")
	if !ok {
		return usage, ""
	}
	args, ok := strings.CutSuffix(rest, ")")
	if !ok {
		return usage, ""
	}
	return name, args
}

func (it Item) withInsertPrefix(prefix string) Item {
	it.Insert = prefix + it.InsertText()
	return it
}

// AppendsSpace reports whether to add a space after insertion in this context.
func (it Item) AppendsSpace(kind Kind) bool {
	if it.noTrailingSpace || it.CursorBack > 0 {
		return false
	}
	switch kind {
	case KindVariable, KindHeaderValue, KindScheme:
		return false
	default:
		return true
	}
}

// PlaceholderRange returns the rune range [start, end) of Placeholder in InsertText.
// It uses the last match to select the value if the same text appears earlier.
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
