package headless

import (
	"io"

	"github.com/unkn0wn-root/resterm/internal/runfmt"
)

// WriteJSON writes r as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		return ErrNilWriter
	}
	rep := toFormatReport(r)
	return runfmt.WriteJSON(w, &rep)
}
