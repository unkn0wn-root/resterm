package headless

import (
	"fmt"
	"io"

	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

// Encode writes the report in the given format.
func (r *Report) Encode(w io.Writer, f Format) error {
	if r == nil {
		return ErrNilReport
	}
	if w == nil {
		return ErrNilWriter
	}

	rep := r.model()
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

// WriteJSON writes r as indented JSON.
func (r *Report) WriteJSON(w io.Writer) error {
	return r.Encode(w, JSON)
}

// WriteJUnit writes r as JUnit XML.
func (r *Report) WriteJUnit(w io.Writer) error {
	return r.Encode(w, JUnit)
}

// WriteText writes r as a text report.
func (r *Report) WriteText(w io.Writer) error {
	return r.Encode(w, Text)
}
