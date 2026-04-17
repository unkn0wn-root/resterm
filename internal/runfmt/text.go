package runfmt

import (
	"fmt"
	"io"
	"strconv"

	"github.com/muesli/termenv"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func WriteText(w io.Writer, rep *Report) error {
	return WriteTextStyled(w, rep, termcolor.Config{})
}

func WriteTextStyled(w io.Writer, rep *Report, color termcolor.Config) error {
	st := newTextStyler(color)
	if _, err := fmt.Fprintf(
		w,
		"%s %d %s from %s with env %s\n",
		st.heading("Running"),
		rep.Total,
		reportTargetLabel(rep),
		reportFileLabel(rep.FilePath),
		reportEnvLabel(rep.EnvName),
	); err != nil {
		return err
	}
	for _, res := range rep.Results {
		if _, err := fmt.Fprintf(
			w,
			"%s %s\n",
			st.resultLabel(resultLabel(res)),
			st.resultLine(resultLine(res)),
		); err != nil {
			return err
		}
		if err := writeTextTargetDetails(w, "  ", res.Target, res.EffectiveTarget, st); err != nil {
			return err
		}
		if err := writeTextProfileDetails(w, "  ", res, st); err != nil {
			return err
		}
		for i, step := range res.Steps {
			if _, err := fmt.Fprintf(
				w,
				"  %s %s %s\n",
				st.index(i+1),
				st.stepLabel(stepLabel(step)),
				st.stepLine(stepLine(step)),
			); err != nil {
				return err
			}
			if err := writeTextTargetDetails(
				w,
				"    ",
				step.Target,
				step.EffectiveTarget,
				st,
			); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(
		w,
		"%s total=%s passed=%s failed=%s skipped=%s\n",
		st.heading("Summary:"),
		st.totalCount(rep.Total),
		st.passCount(rep.Passed),
		st.failCount(rep.Failed),
		st.skipCount(rep.Skipped),
	)
	return err
}

const (
	textColHeading = "#A6A1BB"
	textColSuccess = "#44C25B"
	textColWarn    = "#F25F5C"
	textColCaution = "#FFD46A"
	textColValue   = "#E8E9F0"
)

type textStyler struct {
	cfg termcolor.Config
}

func newTextStyler(cfg termcolor.Config) textStyler {
	return textStyler{cfg: cfg}
}

func (s textStyler) heading(text string) string {
	return s.paint(text, textColHeading, true)
}

func (s textStyler) resultLabel(text string) string {
	switch text {
	case "FAIL", "CANCELED":
		return s.paint(text, textColWarn, true)
	case "SKIP":
		return s.paint(text, textColCaution, true)
	default:
		return s.paint(text, textColSuccess, true)
	}
}

func (s textStyler) stepLabel(text string) string {
	return s.resultLabel(text)
}

func (s textStyler) resultLine(text string) string {
	return s.paint(text, textColValue, true)
}

func (s textStyler) stepLine(text string) string {
	return s.paint(text, textColValue, true)
}

func (s textStyler) detail(label, value string) string {
	return s.detailValue(label, value, true)
}

func (s textStyler) profileDetail(label, value string) string {
	return s.detailValue(label, value, false)
}

func (s textStyler) value(text string) string {
	return s.paint(text, textColValue, false)
}

func (s textStyler) detailValue(label, value string, bold bool) string {
	key := s.paint(label+":", textColHeading, false)
	if value == "" {
		return key
	}
	return key + " " + s.paint(value, textColValue, bold)
}

func (s textStyler) index(n int) string {
	return s.paint(strconv.Itoa(n)+".", textColHeading, false)
}

func (s textStyler) totalCount(n int) string {
	return s.paint(strconv.Itoa(n), textColValue, true)
}

func (s textStyler) passCount(n int) string {
	return s.paint(strconv.Itoa(n), textColSuccess, true)
}

func (s textStyler) failCount(n int) string {
	return s.paint(strconv.Itoa(n), textColWarn, true)
}

func (s textStyler) skipCount(n int) string {
	return s.paint(strconv.Itoa(n), textColCaution, true)
}

func (s textStyler) paint(text, fg string, bold bool) string {
	if !s.cfg.Enabled || text == "" {
		return text
	}
	p := s.profile()
	st := p.String(text)
	if fg != "" {
		st = st.Foreground(p.Color(fg))
	}
	if bold {
		st = st.Bold()
	}
	return st.String()
}

func (s textStyler) profile() termenv.Profile {
	if s.cfg.Profile == termenv.Ascii {
		return termenv.ANSI
	}
	return s.cfg.Profile
}

func writeTextTargetDetails(
	w io.Writer,
	indent string,
	target string,
	effective string,
	st textStyler,
) error {
	source, resolved, ok := targetDetails(target, effective)
	if !ok {
		return nil
	}
	_, err := fmt.Fprintf(
		w,
		"%s%s\n%s%s\n",
		indent,
		st.detail("Source Target", source),
		indent,
		st.detail("Effective Target", resolved),
	)
	return err
}
