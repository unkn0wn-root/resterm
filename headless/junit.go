package headless

import (
	"io"
)

// WriteJUnit writes r as JUnit XML.
func (r *Report) WriteJUnit(w io.Writer) error {
	return r.Encode(w, JUnit)
}
