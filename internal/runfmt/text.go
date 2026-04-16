package runfmt

import (
	"fmt"
	"io"
)

func WriteText(w io.Writer, rep *Report) error {
	if _, err := fmt.Fprintf(
		w,
		"Running %d %s from %s with env %s\n",
		rep.Total,
		reportTargetLabel(rep),
		reportFileLabel(rep.FilePath),
		reportEnvLabel(rep.EnvName),
	); err != nil {
		return err
	}
	for _, res := range rep.Results {
		if _, err := fmt.Fprintf(w, "%s %s\n", resultLabel(res), resultLine(res)); err != nil {
			return err
		}
		for i, step := range res.Steps {
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
		rep.Total,
		rep.Passed,
		rep.Failed,
		rep.Skipped,
	)
	return err
}
