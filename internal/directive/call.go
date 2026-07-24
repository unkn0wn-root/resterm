package directive

import (
	"strings"
	"unicode"
)

// Call keeps the normalized source spelling as well as its canonical name.
// ArgOffset is the zero-based byte offset where Args starts in the original text.
type Call struct {
	Name      Name
	Spelling  Name
	Args      string
	ArgOffset int
}

// Both @name and @name: are accepted, with or without a space before the value.
// Unknown names are returned unchanged so callers can decide whether they are
// valid in the current part of the file.
func Parse(text string) (Call, bool) {
	body, ok := strings.CutPrefix(text, "@")
	if !ok {
		return Call{}, false
	}

	lead := len(body) - len(strings.TrimLeftFunc(body, unicode.IsSpace))
	rest := body[lead:]
	end := strings.IndexFunc(rest, IsArgSep)
	if end < 0 {
		end = len(rest)
	}
	spelling := Name(strings.ToLower(rest[:end]))
	if spelling == "" {
		return Call{}, false
	}

	// Trimming the head with the same predicate that found it keeps ArgOffset on
	// the first byte of Args. Only spaces come off the tail, a trailing colon
	// belongs to the value.
	tail := rest[end:]
	args := strings.TrimLeftFunc(tail, IsArgSep)
	gap := len(tail) - len(args)
	return Call{
		Name:      spelling.Canonical(),
		Spelling:  spelling,
		Args:      strings.TrimRightFunc(args, unicode.IsSpace),
		ArgOffset: 1 + lead + end + gap,
	}, true
}

// The editor works rune by rune as you type, so it cannot just call Parse on a
// half finished line. It uses this instead and has to stay in step with Parse,
// which is why Parse locates and trims the argument with it too. Any space
// Unicode calls a space counts, not just the ASCII ones.
func IsArgSep(r rune) bool {
	return r == ':' || unicode.IsSpace(r)
}

// CutToken splits off the first whitespace separated token and leaves it
// unchanged for callers where case and punctuation matter.
func CutToken(text string) (string, string) {
	tr := strings.TrimSpace(text)
	i := strings.IndexFunc(tr, unicode.IsSpace)
	if i < 0 {
		return tr, ""
	}
	return tr[:i], strings.TrimSpace(tr[i:])
}

// CutKey prepares the first token for lookup by lowercasing it and dropping a
// trailing colon.
func CutKey(text string) (string, string) {
	tok, rest := CutToken(text)
	return strings.ToLower(strings.TrimRight(tok, ":")), rest
}
