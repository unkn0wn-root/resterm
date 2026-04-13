package headless

import (
	"fmt"
	"io"
)

// WriteText writes r as a text report.
// If r is nil, WriteText is a no-op. If w is nil, WriteText returns ErrNilWriter.
func (r *Report) WriteText(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		return ErrNilWriter
	}
	if _, err := fmt.Fprintf(
		w,
		"Running %d %s from %s with env %s\n",
		r.Total,
		reportTargetLabel(r),
		reportFileLabel(r.FilePath),
		reportEnvLabel(r.EnvName),
	); err != nil {
		return err
	}
	for _, item := range r.Results {
		if _, err := fmt.Fprintf(w, "%s %s\n", resultLabel(item), resultLine(item)); err != nil {
			return err
		}
		for i, step := range item.Steps {
			if _, err := fmt.Fprintf(
				w,
				"  %d. %s %s\n",
				i+1,
				stepLabel(step),
				stepLine(step),
			); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(
		w,
		"Summary: total=%d passed=%d failed=%d skipped=%d\n",
		r.Total,
		r.Passed,
		r.Failed,
		r.Skipped,
	)
	return err
}
