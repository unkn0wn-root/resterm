package headless

import (
	"io"

	"github.com/unkn0wn-root/resterm/internal/runfmt"
)

// WriteJUnit writes r as JUnit XML.
func (r *Report) WriteJUnit(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		return ErrNilWriter
	}
	rep := toFormatReport(r)
	return runfmt.WriteJUnit(w, &rep)
}
