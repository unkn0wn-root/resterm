package runfmt

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// TextPainter decorates text segments in the shared text formatter.
// Implementations can apply ANSI styling or return the text unchanged.
type TextPainter interface {
	PaintText(text, fg string, bold bool) string
}

type TextPalette struct {
	Heading string
	Success string
	Warn    string
	Caution string
	Value   string
}

type textPaletteProvider interface {
	TextPalette() TextPalette
}

// TextPaintFunc adapts a function to TextPainter.
type TextPaintFunc func(text, fg string, bold bool) string

func (fn TextPaintFunc) PaintText(text, fg string, bold bool) string {
	if fn == nil {
		return text
	}
	return fn(text, fg, bold)
}

func WriteText(w io.Writer, rep *Report) error {
	return writeText(w, rep, plainTextPainter{})
}

func WriteTextStyled(w io.Writer, rep *Report, painter TextPainter) error {
	return writeText(w, rep, painter)
}

func writeText(w io.Writer, rep *Report, painter TextPainter) error {
	st := newTextStyler(painter)
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
		if err := writeTextErrorDetails(w, "  ", res.ErrorDetail, res.ScriptErrorDetail, st); err != nil {
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
			if err := writeTextErrorDetails(
				w,
				"    ",
				step.ErrorDetail,
				step.ScriptErrorDetail,
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

func writeTextErrorDetails(
	w io.Writer,
	indent string,
	errDetail *ErrorDetail,
	scriptDetail *ErrorDetail,
	st textStyler,
) error {
	if err := writeTextErrorDetail(w, indent, "Error", errDetail, st); err != nil {
		return err
	}
	return writeTextErrorDetail(w, indent, "Script Error", scriptDetail, st)
}

func writeTextErrorDetail(
	w io.Writer,
	indent string,
	label string,
	detail *ErrorDetail,
	st textStyler,
) error {
	body := errorDetailText(detail, "")
	if body == "" {
		return nil
	}
	_, err := fmt.Fprintf(
		w,
		"%s%s\n%s\n",
		indent,
		st.detail(label, ""),
		indentBlock(styleErrorDetailBlock(body, st), indent+"  "),
	)
	return err
}

func styleErrorDetailBlock(text string, st textStyler) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = st.value(line)
	}
	return strings.Join(lines, "\n")
}

func indentBlock(text, prefix string) string {
	if text == "" || prefix == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

const (
	textColHeading = "#A6A1BB"
	textColSuccess = "#44C25B"
	textColWarn    = "#F25F5C"
	textColCaution = "#FFD46A"
	textColValue   = "#E8E9F0"
)

type textStyler struct {
	painter TextPainter
	pal     TextPalette
}

func newTextStyler(painter TextPainter) textStyler {
	if painter == nil {
		painter = plainTextPainter{}
	}
	pal := defaultTextPalette()
	if p, ok := painter.(textPaletteProvider); ok {
		if got := p.TextPalette(); got != (TextPalette{}) {
			pal = mergeTextPalette(pal, got)
		}
	}
	return textStyler{painter: painter, pal: pal}
}

func (s textStyler) heading(text string) string {
	return s.paint(text, s.pal.Heading, true)
}

func (s textStyler) resultLabel(text string) string {
	switch text {
	case "FAIL", "CANCELED":
		return s.paint(text, s.pal.Warn, true)
	case "SKIP":
		return s.paint(text, s.pal.Caution, true)
	default:
		return s.paint(text, s.pal.Success, true)
	}
}

func (s textStyler) stepLabel(text string) string {
	return s.resultLabel(text)
}

func (s textStyler) resultLine(text string) string {
	return s.paint(text, s.pal.Value, true)
}

func (s textStyler) stepLine(text string) string {
	return s.paint(text, s.pal.Value, true)
}

func (s textStyler) detail(label, value string) string {
	return s.detailValue(label, value, true)
}

func (s textStyler) profileDetail(label, value string) string {
	return s.detailValue(label, value, false)
}

func (s textStyler) value(text string) string {
	return s.paint(text, s.pal.Value, false)
}

func (s textStyler) detailValue(label, value string, bold bool) string {
	key := s.paint(label+":", s.pal.Heading, false)
	if value == "" {
		return key
	}
	return key + " " + s.paint(value, s.pal.Value, bold)
}

func (s textStyler) index(n int) string {
	return s.paint(strconv.Itoa(n)+".", s.pal.Heading, false)
}

func (s textStyler) totalCount(n int) string {
	return s.paint(strconv.Itoa(n), s.pal.Value, true)
}

func (s textStyler) passCount(n int) string {
	return s.paint(strconv.Itoa(n), s.pal.Success, true)
}

func (s textStyler) failCount(n int) string {
	return s.paint(strconv.Itoa(n), s.pal.Warn, true)
}

func (s textStyler) skipCount(n int) string {
	return s.paint(strconv.Itoa(n), s.pal.Caution, true)
}

func (s textStyler) paint(text, fg string, bold bool) string {
	if text == "" {
		return text
	}
	return s.painter.PaintText(text, fg, bold)
}

type plainTextPainter struct{}

func (plainTextPainter) PaintText(text, _ string, _ bool) string {
	return text
}

func defaultTextPalette() TextPalette {
	return TextPalette{
		Heading: textColHeading,
		Success: textColSuccess,
		Warn:    textColWarn,
		Caution: textColCaution,
		Value:   textColValue,
	}
}

func mergeTextPalette(base, over TextPalette) TextPalette {
	if over.Heading != "" {
		base.Heading = over.Heading
	}
	if over.Success != "" {
		base.Success = over.Success
	}
	if over.Warn != "" {
		base.Warn = over.Warn
	}
	if over.Caution != "" {
		base.Caution = over.Caution
	}
	if over.Value != "" {
		base.Value = over.Value
	}
	return base
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
