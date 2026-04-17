package headless

import (
	"fmt"
	"io"

	"github.com/unkn0wn-root/resterm/internal/runfmt"
)

// Encode writes the report in the given format.
func (r *Report) Encode(w io.Writer, f Format) error {
	if r == nil {
		return ErrNilReport
	}
	if w == nil {
		return ErrNilWriter
	}

	rep := toFormatReport(r)
	switch f {
	case JSON:
		return runfmt.WriteJSON(w, &rep)
	case JUnit:
		return runfmt.WriteJUnit(w, &rep)
	case Text:
		return runfmt.WriteText(w, &rep)
	default:
		return fmt.Errorf("headless: unsupported format %d", int(f))
	}
}
