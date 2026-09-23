package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	profileWideWidth    = 96
	profileCompactWidth = 56
	profileMaxFailures  = 20
	profileColumnGap    = "   "
	profileSep          = " · "
)

type profileStatsView struct {
	title string
	env   string
	snap  core.ProfileSnapshot
}

type profileKPI struct {
	label string
	value string
}

func (v *profileStatsView) render(width int, pal statsPalette, th theme.Theme) string {
	lines := v.header(width, pal, th)
	lines = append(lines, "")
	lines = append(lines, v.kpis(width, pal)...)
	lines = append(lines, "")
	if width >= profileWideWidth {
		lw := (width - len(profileColumnGap)) * 3 / 5
		rw := width - lw - len(profileColumnGap)
		lines = append(lines, joinProfileColumns(v.latency(lw, pal, th), v.failures(rw, pal), lw)...)
	} else {
		lines = append(lines, v.latency(width, pal, th)...)
		lines = append(lines, "")
		lines = append(lines, v.failures(width, pal)...)
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
	items := v.kpiItems()
	if width < profileCompactWidth {
		lines := make([]string, len(items))
		for i, it := range items {
			lines[i] = pal.Label.Render(fmt.Sprintf("%-9s", it.label)) + pal.Value.Render(it.value)
		}
		return lines
	}
	var head, vals strings.Builder
	for i, it := range items {
		label, value := it.label, it.value
		if i < len(items)-1 {
			w := max(len(label), ansi.StringWidth(value)) + len(profileColumnGap)
			label, value = padStyled(label, w), padStyled(value, w)
		}
		head.WriteString(pal.Heading.Render(label))
		vals.WriteString(pal.Value.Render(value))
	}
	return []string{head.String(), vals.String()}
}

func (v *profileStatsView) kpiItems() []profileKPI {
	p := v.snap.Progress
	st := v.snap.Stats
	success := "-"
	if p.Measured > 0 {
		success = fmt.Sprintf("%d%%", p.Passed*100/p.Measured)
	}
	rate := "-"
	if r := v.snap.WallRate(); r > 0 {
		rate = strconv.FormatFloat(r, 'f', 1, 64) + "/s wall"
	}
	return []profileKPI{
		{"SUCCESS", success},
		{"P50", formatDurationShort(st.Percentiles[50])},
		{"P95", formatDurationShort(st.Percentiles[95])},
		{"P99", formatDurationShort(st.Percentiles[99])},
		{"RATE", rate},
	}
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
			msg = "Percentiles and the distribution appear when the run ends."
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
		room := max(width-ansi.StringWidth(label)-2, 1)
		lines = append(lines, style.Render(label+": ")+pal.Value.Render(ansi.Truncate(detail, room, "…")))
	}
	if n := len(v.snap.Failures) - len(shown); n > 0 {
		lines = append(lines, pal.Message.Render(fmt.Sprintf("%d more not shown", n)))
	}
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

func joinProfileColumns(left, right []string, lw int) []string {
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		var l, r string
		if i < len(left) {
			l = ansi.Truncate(left[i], lw, "…")
		}
		if i < len(right) {
			r = right[i]
		}
		if r == "" {
			out[i] = l
			continue
		}
		out[i] = padStyled(l, lw) + profileColumnGap + r
	}
	return out
}
