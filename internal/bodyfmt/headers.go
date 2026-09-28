package bodyfmt

import (
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/termtext"
)

type HeaderField struct {
	Name  string
	Value string
}

func (f HeaderField) String() string {
	if f.Value == "" {
		return f.Name + ":"
	}
	return f.Name + ": " + f.Value
}

// HeaderFields returns one field per value, sorted by name. Values keep the
// order they arrived in and are never joined, since Set-Cookie values cannot be.
func HeaderFields(headers http.Header) []HeaderField {
	if len(headers) == 0 {
		return nil
	}

	out := make([]HeaderField, 0, len(headers))
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		for _, val := range headers[name] {
			out = append(out, HeaderField{Name: termtext.Row(name), Value: termtext.Row(val)})
		}
	}
	return out
}

func FormatHeaders(headers http.Header) string {
	fields := HeaderFields(headers)
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		lines = append(lines, field.String())
	}
	return strings.Join(lines, "\n")
}
