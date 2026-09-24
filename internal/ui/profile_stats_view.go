package ui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	profileCompactWidth = 56
	profileMaxFailures  = 20
	profileFailureLines = 3
	profileColumnGap    = "   "
	profileSep          = " · "
)

type profileStatsView struct {
	title string
	env   string
	snap  core.ProfileSnapshot
	base  *core.ProfileSnapshot
}

func (v *profileStatsView) render(width int, pal statsPalette, th theme.Theme) string {
	var lines []string
	for _, sec := range [][]string{
		v.header(width, pal, th),
		v.kpis(width, pal),
		v.latency(width, pal, th),
		v.responses(width, pal),
		v.failures(width, pal),
	} {
		if len(sec) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, sec...)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return strings.Join(lines, "\n")
}

func (v *profileStatsView) header(width int, pal statsPalette, th theme.Theme) []string {
	p := v.snap.Progress
	word := strings.ToUpper(p.Status.String())
	lead := "PROFILE  "
	room := max(width-len(lead)-len(word)-2, 1)
	title := ansi.Truncate(v.title, room, "…")
	gap := max(width-len(lead)-ansi.StringWidth(title)-len(word), 2)
	lines := []string{
		pal.Title.Render(lead) + pal.Value.Render(title) + strings.Repeat(" ", gap) +
			profileStatusStyle(p.Status, pal, th).Render(word),
	}
	for _, l := range fitProfileParts(v.metaParts(), profileSep, width) {
		lines = append(lines, pal.SubLabel.Render(l))
	}
	lines = append(lines, v.progressBar(width, pal, th))
	switch p.Status {
	case core.ProfileCanceled, core.ProfileSkipped, core.ProfileError:
		lines = append(lines, profileStatusStyle(p.Status, pal, th).Render(v.snap.Summary()))
	}
	return lines
}

func (v *profileStatsView) metaParts() []string {
	p := v.snap.Progress
	var parts []string
	if v.env != "" {
		parts = append(parts, v.env)
	}
	parts = append(parts, fmt.Sprintf("%d measured", p.Count))
	if p.Warmup > 0 {
		parts = append(parts, fmt.Sprintf("%d warmup", p.Warmup))
	}
	if v.snap.Delay > 0 {
		parts = append(parts, formatDurationShort(v.snap.Delay)+" delay")
	}
	if p.Elapsed > 0 {
		parts = append(parts, formatDurationShort(p.Elapsed)+" wall")
	}
	return parts
}

func (v *profileStatsView) progressBar(width int, pal statsPalette, th theme.Theme) string {
	p := v.snap.Progress
	label, done, total := "Measured", p.Measured, p.Count
	if p.Status == core.ProfileRunning && p.WarmupDone < p.Warmup {
		label, done, total = "Warmup", p.WarmupDone, p.Warmup
	}
	count := fmt.Sprintf("%d/%d", done, total)
	size := min(max(width-len(label)-len(count)-4, 4), 30)
	fill := 0
	if total > 0 {
		fill = min(done*size/total, size)
	}
	return pal.Label.Render(label) + "  " + renderMeter(fill, size, profileStatusStyle(p.Status, pal, th)) + "  " +
		pal.Value.Render(count)
}

func (v *profileStatsView) kpis(width int, pal statsPalette) []string {
	cells := v.kpiCells()
	lines := kpiGrid(cells, pal, width)
	if lines == nil {
		lines = kpiList(cells, pal)
	}
	if v.base == nil {
		return lines
	}
	parts := []string{
		"vs run at " + historyTimestampLabel(v.base.Ended, time.Now()),
		fmt.Sprintf("%d measured", v.base.Progress.Measured),
	}
	for _, l := range fitProfileParts(parts, profileSep, width) {
		lines = append(lines, pal.SubLabel.Render(l))
	}
	return lines
}

func kpiGrid(cells []kpiCell, pal statsPalette, width int) []string {
	widths := make([]int, len(cells))
	total := len(profileColumnGap) * (len(cells) - 1)
	for i, c := range cells {
		widths[i] = max(len(c.label), ansi.StringWidth(c.value), ansi.StringWidth(c.delta))
		total += widths[i]
	}
	if total > width {
		return nil
	}
	var head, vals, deltas strings.Builder
	for i, c := range cells {
		w := widths[i] + len(profileColumnGap)
		if i == len(cells)-1 {
			w = 0
		}
		head.WriteString(pal.Heading.Render(padStyled(c.label, w)))
		vals.WriteString(pal.Value.Render(padStyled(c.value, w)))
		deltas.WriteString(c.trend.style(pal).Render(padStyled(c.delta, w)))
	}
	lines := []string{head.String(), vals.String()}
	if slices.ContainsFunc(cells, func(c kpiCell) bool { return c.delta != "" }) {
		lines = append(lines, deltas.String())
	}
	return lines
}

func kpiList(cells []kpiCell, pal statsPalette) []string {
	vw := 0
	for _, c := range cells {
		if c.delta != "" {
			vw = max(vw, ansi.StringWidth(c.value))
		}
	}
	lines := make([]string, len(cells))
	for i, c := range cells {
		label := pal.Label.Render(fmt.Sprintf("%-9s", c.label))
		if c.delta == "" {
			lines[i] = label + pal.Value.Render(c.value)
			continue
		}
		lines[i] = label + pal.Value.Render(padStyled(c.value, vw)) + "  " + c.trend.style(pal).Render(c.delta)
	}
	return lines
}

func (v *profileStatsView) latency(width int, pal statsPalette, th theme.Theme) []string {
	st := v.snap.Stats
	note := " successful measured requests only"
	if width < profileCompactWidth {
		note = " successful only"
	}
	lines := []string{pal.Heading.Render("LATENCY") + pal.SubLabel.Render(note)}
	if st.Count == 0 {
		msg := "No successful measured requests."
		if v.snap.Progress.Status == core.ProfileRunning {
			msg = "No successful measured requests yet."
		}
		return append(lines, pal.Message.Render(msg))
	}

	ranges := make([]string, len(st.Histogram))
	marks := percentileMarks(st)
	rw, cw, mw, maxCount := 0, 0, 0, 0
	for i, b := range st.Histogram {
		ranges[i] = formatDurationShort(b.From)
		if b.To != b.From {
			ranges[i] += "-" + formatDurationShort(b.To)
		}
		rw = max(rw, len(ranges[i]))
		cw = max(cw, len(strconv.Itoa(b.Count)))
		maxCount = max(maxCount, b.Count)
		if marks[i] != "" {
			mw = max(mw, len(marks[i])+2)
		}
	}
	barMax := 40
	if width < profileCompactWidth {
		barMax = 12
	}
	size := min(max(width-rw-cw-mw-10, 4), barMax)
	for i, b := range st.Histogram {
		fill := 0
		if maxCount > 0 {
			fill = (b.Count*size + maxCount - 1) / maxCount
		}
		line := pal.Label.Render(padStyled(ranges[i], rw)) + "  " +
			renderMeter(fill, size, latFg(th, b.From+(b.To-b.From)/2)) + "  " +
			pal.Value.Render(fmt.Sprintf("%*d", cw, b.Count)) + "  " +
			pal.SubLabel.Render(fmt.Sprintf("%3d%%", b.Count*100/st.Count))
		if marks[i] != "" {
			line += pal.SubLabel.Render("  " + marks[i])
		}
		lines = append(lines, line)
	}

	kv := func(label, value string) string {
		return pal.SubLabel.Render(label+" ") + pal.Value.Render(value)
	}
	parts := []string{
		kv("min", formatDurationShort(st.Min)),
		kv("mean", formatDurationShort(st.Mean)),
		kv("max", formatDurationShort(st.Max)),
		kv("stddev", formatDurationShort(st.StdDev)),
	}
	if r := v.snap.ActiveRate(); r > 0 {
		parts = append(parts, kv("active", strconv.FormatFloat(r, 'f', 1, 64)+"/s"))
	}
	return append(lines, fitProfileParts(parts, pal.SubLabel.Render(profileSep), width)...)
}

func (v *profileStatsView) responses(width int, pal statsPalette) []string {
	codes := v.snap.StatusCodes
	if len(codes) == 0 {
		return nil
	}
	order := slices.SortedFunc(maps.Keys(codes), func(a, b int) int {
		return cmp.Or(cmp.Compare(codes[b], codes[a]), cmp.Compare(a, b))
	})
	parts := make([]string, len(order))
	for i, code := range order {
		name := strconv.Itoa(code)
		if code == 0 {
			name = "no response"
		}
		parts[i] = statusCodeStyle(code, pal).Render(name) + pal.Value.Render(" ×"+strconv.Itoa(codes[code]))
	}
	lines := []string{pal.Heading.Render("RESPONSES") + pal.SubLabel.Render(" measured requests")}
	return append(lines, fitProfileParts(parts, pal.SubLabel.Render(profileSep), width)...)
}

func statusCodeStyle(code int, pal statsPalette) lipgloss.Style {
	switch {
	case code == 0 || code >= 400:
		return pal.Warn
	case code >= 300:
		return pal.Caution
	default:
		return pal.Success
	}
}

func (v *profileStatsView) failures(width int, pal statsPalette) []string {
	p := v.snap.Progress
	head, fails, warm := pal.Heading, pal.Value, pal.Value
	if p.Failed > 0 {
		head, fails = pal.HeadingWarn, pal.Warn
	}
	if p.WarmupFailed > 0 {
		warm = pal.Caution
	}
	lines := []string{
		head.Render("FAILURES"),
		pal.Label.Render("Failures ") + fails.Render(strconv.Itoa(p.Failed)) +
			pal.SubLabel.Render(profileSep) +
			pal.Label.Render("Warmup warnings ") + warm.Render(strconv.Itoa(p.WarmupFailed)),
	}
	if v.snap.Legacy && len(v.snap.Failures) == 0 && p.Failed+p.WarmupFailed > 0 {
		lines = append(lines, pal.Message.Render("Failure details are unavailable for this older entry."))
	}
	shown := v.snap.Failures[:min(len(v.snap.Failures), profileMaxFailures)]
	for _, f := range shown {
		label, style := fmt.Sprintf("Run %d", f.Iteration), pal.Warn
		if f.Warmup {
			label, style = fmt.Sprintf("Warmup %d", f.Iteration), pal.Caution
		}
		detail := f.Reason
		if f.Duration > 0 {
			detail += profileSep + formatDurationShort(f.Duration)
		}
		indent := ansi.StringWidth(label) + 2
		for i, l := range wrapProfileText(detail, max(width-indent, 1), profileFailureLines) {
			lead := strings.Repeat(" ", indent)
			if i == 0 {
				lead = style.Render(label + ": ")
			}
			lines = append(lines, lead+pal.Value.Render(l))
		}
	}
	if n := len(v.snap.Failures) - len(shown); n > 0 {
		lines = append(lines, pal.Message.Render(fmt.Sprintf("%d more not shown", n)))
	}
	return lines
}

func wrapProfileText(s string, width, limit int) []string {
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	if len(lines) <= limit {
		return lines
	}
	lines = lines[:limit]
	lines[limit-1] = ansi.Truncate(lines[limit-1], width-1, "") + "…"
	return lines
}

func fitProfileParts(parts []string, sep string, width int) []string {
	var lines []string
	var cur string
	for _, part := range parts {
		switch {
		case cur == "":
			cur = part
		case ansi.StringWidth(cur+sep+part) <= width:
			cur += sep + part
		default:
			lines = append(lines, cur)
			cur = part
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// percentileMarks labels the bucket each percentile falls in. Buckets end on
// rounded bounds, so a value past the last one belongs to the last bucket.
func percentileMarks(st analysis.LatencyStats) []string {
	marks := make([]string, len(st.Histogram))
	for _, p := range []int{50, 95, 99} {
		v, ok := st.Percentiles[p]
		if !ok || len(marks) == 0 {
			continue
		}
		i := slices.IndexFunc(st.Histogram, func(b analysis.HistogramBucket) bool { return v <= b.To })
		if i < 0 {
			i = len(marks) - 1
		}
		marks[i] = strings.TrimSpace(marks[i] + " p" + strconv.Itoa(p))
	}
	return marks
}

func profileStatusStyle(st core.ProfileStatus, pal statsPalette, th theme.Theme) lipgloss.Style {
	switch st {
	case core.ProfilePass:
		return pal.Success
	case core.ProfileFail, core.ProfileError:
		return pal.Warn
	case core.ProfileCanceled, core.ProfileSkipped:
		return pal.Caution
	default:
		return lipgloss.NewStyle().Foreground(th.HeaderValue.GetForeground())
	}
}
