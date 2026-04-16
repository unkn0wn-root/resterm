package headless

import (
	"io"

	"github.com/unkn0wn-root/resterm/internal/runfmt"
)

// WriteText writes r as a text report.
func (r *Report) WriteText(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		return ErrNilWriter
	}
	rep := toFormatReport(r)
	return runfmt.WriteText(w, &rep)
}
