package bodyfmt

import (
	"maps"
	"net/http"
	"slices"
	"strings"
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

// HeaderFields flattens headers into one field per name, sorted by name and by
// value so repeated renders of the same response stay stable.
func HeaderFields(headers http.Header) []HeaderField {
	if len(headers) == 0 {
		return nil
	}

	out := make([]HeaderField, 0, len(headers))
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		values := slices.Sorted(slices.Values(headers[name]))
		out = append(out, HeaderField{Name: name, Value: strings.Join(values, ", ")})
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
