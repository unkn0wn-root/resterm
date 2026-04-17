package headless

import (
	"io"
)

// WriteText writes r as a text report.
func (r *Report) WriteText(w io.Writer) error {
	return r.Encode(w, Text)
}
