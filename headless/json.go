package headless

import (
	"io"
)

// WriteJSON writes r as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	return r.Encode(w, JSON)
}
