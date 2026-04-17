package runfmt

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type textProfileRow struct {
	label string
	value string
}

const textProfileHistWid = 22

func writeTextProfileDetails(w io.Writer, indent string, res Result, st textStyler) error {
	p := res.Profile
	if res.Kind != "profile" || p == nil {
		return nil
	}

	rows := textProfileRows(p)
	if len(rows) == 0 && len(p.Histogram) == 0 && len(p.Failures) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "%s%s\n", indent, st.heading("Profile:")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(
			w,
			"%s  %s\n",
			indent,
			st.profileDetail(row.label, row.value),
		); err != nil {
			return err
		}
	}
	if err := writeTextProfileHistogram(w, indent, p, st); err != nil {
		return err
	}

	if len(p.Failures) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(
		w,
		"%s  %s\n",
		indent,
		st.paint("Failures:", textColWarn, true),
	); err != nil {
		return err
	}
	for _, fail := range p.Failures {
		if _, err := fmt.Fprintf(
			w,
			"%s    %s\n",
			indent,
			st.value("- "+textProfileFailure(fail)),
		); err != nil {
			return err
		}
	}
	return nil
}

func textProfileRows(p *Profile) []textProfileRow {
	rows := make([]textProfileRow, 0, 6)
	if v := textProfilePlan(p); v != "" {
		rows = append(rows, textProfileRow{label: "Plan", value: v})
	}
	if v := textProfileRuns(p); v != "" {
		rows = append(rows, textProfileRow{label: "Runs", value: v})
	}
	if v := textProfileSuccess(p); v != "" {
		rows = append(rows, textProfileRow{label: "Success", value: v})
	}
	if p.Delay > 0 {
		rows = append(rows, textProfileRow{
			label: "Delay",
			value: textProfileDuration(p.Delay) + " between runs",
		})
	}
	if v := textProfileLatency(p); v != "" {
		rows = append(rows, textProfileRow{label: "Latency", value: v})
	}
	if v := textProfileStats(p); v != "" {
		rows = append(rows, textProfileRow{label: "Stats", value: v})
	}
	return rows
}

func writeTextProfileHistogram(
	w io.Writer,
	indent string,
	p *Profile,
	st textStyler,
) error {
	if p == nil || len(p.Histogram) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "%s  %s\n", indent, st.heading("Histogram:")); err != nil {
		return err
	}
	for _, row := range textProfileHistogramRows(p) {
		if _, err := fmt.Fprintf(
			w,
			"%s    %s%s%s\n",
			indent,
			st.value(row.head),
			st.paint(row.bar, row.color, false),
			st.value(row.tail),
		); err != nil {
			return err
		}
	}
	return nil
}

func textProfilePlan(p *Profile) string {
	parts := make([]string, 0, 2)
	if p.Count > 0 {
		parts = append(parts, fmt.Sprintf("%d measured", p.Count))
	}
	if p.Warmup > 0 {
		parts = append(parts, fmt.Sprintf("%d warmup", p.Warmup))
	}
	return strings.Join(parts, " | ")
}

func textProfileRuns(p *Profile) string {
	if p.TotalRuns == 0 && p.SuccessfulRuns == 0 && p.FailedRuns == 0 && p.WarmupRuns == 0 {
		return ""
	}

	parts := []string{
		fmt.Sprintf("%d total", p.TotalRuns),
		fmt.Sprintf("%d success", p.SuccessfulRuns),
		fmt.Sprintf("%d failure", p.FailedRuns),
	}
	if p.WarmupRuns > 0 {
		parts = append(parts, fmt.Sprintf("%d warmup", p.WarmupRuns))
	}
	return strings.Join(parts, " | ")
}

func textProfileSuccess(p *Profile) string {
	n := p.SuccessfulRuns + p.FailedRuns
	if n <= 0 {
		return ""
	}
	rate := (float64(p.SuccessfulRuns) / float64(n)) * 100
	return fmt.Sprintf("%.0f%% (%d/%d)", rate, p.SuccessfulRuns, n)
}

func textProfileLatency(p *Profile) string {
	if p.Latency == nil || p.Latency.Count == 0 {
		return ""
	}

	lat := p.Latency
	parts := []string{
		fmt.Sprintf("%d samples", lat.Count),
		"min " + textProfileDuration(lat.Min),
	}
	if d, ok := textProfilePercentile(p.Percentiles, 50); ok {
		parts = append(parts, "p50 "+textProfileDuration(d))
	} else {
		parts = append(parts, "median "+textProfileDuration(lat.Median))
	}
	for _, pct := range analysis.DefaultProfileTailPercentiles() {
		if d, ok := textProfilePercentile(p.Percentiles, pct); ok {
			parts = append(parts, fmt.Sprintf("p%d %s", pct, textProfileDuration(d)))
		}
	}
	parts = append(parts, "max "+textProfileDuration(lat.Max))
	return strings.Join(parts, " | ")
}

func textProfileStats(p *Profile) string {
	if p.Latency == nil || p.Latency.Count == 0 {
		return ""
	}
	lat := p.Latency
	parts := []string{
		"mean " + textProfileDuration(lat.Mean),
		"median " + textProfileDuration(lat.Median),
		"stddev " + textProfileDuration(lat.StdDev),
	}
	return strings.Join(parts, " | ")
}

type textProfileHistLayout struct {
	from []string
	to   []string
	cnt  []string
	pct  []string
	fw   int
	tw   int
	cw   int
	pw   int
	mx   int
	tot  int
}

type textProfileHistRow struct {
	head  string
	bar   string
	tail  string
	color string
}

func textProfileHistogramRows(p *Profile) []textProfileHistRow {
	hl := textProfileHistogramLayout(p.Histogram)
	rows := make([]textProfileHistRow, 0, len(p.Histogram))
	p50, p90, ok := textProfileHistogramThresholds(p)
	for i, bin := range p.Histogram {
		rows = append(rows, textProfileHistRow{
			head: fmt.Sprintf(
				"%-*s - %-*s | ",
				hl.fw,
				hl.from[i],
				hl.tw,
				hl.to[i],
			),
			bar:   textProfileHistogramBar(bin.Count, hl.mx),
			tail:  fmt.Sprintf(" (%*s, %*s)", hl.cw, hl.cnt[i], hl.pw, hl.pct[i]),
			color: textProfileHistogramColor(bin, p50, p90, ok),
		})
	}
	return rows
}

func textProfileHistogramLayout(bins []HistBin) textProfileHistLayout {
	hl := textProfileHistLayout{
		from: make([]string, len(bins)),
		to:   make([]string, len(bins)),
		cnt:  make([]string, len(bins)),
		pct:  make([]string, len(bins)),
	}
	for i, bin := range bins {
		hl.from[i] = textProfileDuration(bin.From)
		hl.to[i] = textProfileDuration(bin.To)
		hl.cnt[i] = strconv.Itoa(bin.Count)
		hl.fw = max(hl.fw, len(hl.from[i]))
		hl.tw = max(hl.tw, len(hl.to[i]))
		hl.cw = max(hl.cw, len(hl.cnt[i]))
		hl.tot += bin.Count
		if bin.Count > hl.mx {
			hl.mx = bin.Count
		}
	}
	if hl.mx < 1 {
		hl.mx = 1
	}
	if hl.tot < 1 {
		hl.tot = 1
	}
	for i, bin := range bins {
		hl.pct[i] = textProfileHistogramPercent(bin.Count, hl.tot)
		hl.pw = max(hl.pw, len(hl.pct[i]))
	}
	return hl
}

func textProfileHistogramBar(n, max int) string {
	if max < 1 {
		max = 1
	}
	if n < 0 {
		n = 0
	}
	fill := 0
	if n > 0 {
		fill = int(math.Round(float64(n) / float64(max) * float64(textProfileHistWid)))
		if fill == 0 {
			fill = 1
		}
	}
	if fill > textProfileHistWid {
		fill = textProfileHistWid
	}
	return strings.Repeat("#", fill) + strings.Repeat(".", textProfileHistWid-fill)
}

func textProfileHistogramPercent(n, tot int) string {
	if tot <= 0 {
		return "0%"
	}
	return fmt.Sprintf("%.1f%%", (float64(n)/float64(tot))*100)
}

func textProfileHistogramThresholds(p *Profile) (time.Duration, time.Duration, bool) {
	if p == nil {
		return 0, 0, false
	}
	p50, ok50 := textProfilePercentile(p.Percentiles, 50)
	if !ok50 && p.Latency != nil && p.Latency.Median > 0 {
		p50 = p.Latency.Median
		ok50 = true
	}
	p90, ok90 := textProfilePercentile(p.Percentiles, 90)
	if ok50 && ok90 {
		return p50, p90, true
	}
	return 0, 0, false
}

func textProfileHistogramColor(bin HistBin, p50, p90 time.Duration, ok bool) string {
	if !ok {
		return textColValue
	}
	switch {
	case bin.To <= p50:
		return textColSuccess
	case bin.To >= p90 || bin.From >= p90:
		return textColWarn
	default:
		return textColCaution
	}
}

func textProfilePercentile(vals []Percentile, pct int) (time.Duration, bool) {
	for _, val := range vals {
		if val.Percentile == pct {
			return val.Value, true
		}
	}
	return 0, false
}

func textProfileFailure(fail ProfileFailure) string {
	label := fmt.Sprintf("Run %d", fail.Iteration)
	if fail.Warmup {
		label = fmt.Sprintf("Warmup %d", fail.Iteration)
	}

	msg := str.Trim(fail.Reason)
	meta := textProfileFailureMeta(fail)
	switch {
	case msg != "" && meta != "":
		msg = fmt.Sprintf("%s [%s]", msg, meta)
	case msg == "" && meta != "":
		msg = meta
	case msg == "":
		msg = "failed"
	}
	return label + ": " + msg
}

func textProfileFailureMeta(fail ProfileFailure) string {
	parts := make([]string, 0, 2)
	if status := str.Trim(fail.Status); status != "" {
		parts = append(parts, status)
	} else if fail.StatusCode > 0 {
		parts = append(parts, strconv.Itoa(fail.StatusCode))
	}
	if fail.Duration > 0 {
		parts = append(parts, textProfileDuration(fail.Duration))
	}
	return strings.Join(parts, " | ")
}

func textProfileDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Microsecond {
		return d.String()
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d/time.Microsecond)
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Round(time.Millisecond).String()
}
