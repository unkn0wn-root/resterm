package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

func renderHistogram(bins []analysis.HistogramBucket, indent string) string {
	if len(bins) == 0 {
		return ""
	}
	if indent == "" {
		indent = histogramDefaultIndent
	}

	layout := buildHistogramRenderLayout(bins)
	rowIndent := indent + histogramDefaultIndent
	var builder strings.Builder

	for i, bucket := range bins {
		builder.WriteString(rowIndent)
		builder.WriteString(fmt.Sprintf("%-*s", layout.fromWidth, layout.from[i]))
		builder.WriteString(" – ")
		builder.WriteString(fmt.Sprintf("%-*s", layout.toWidth, layout.to[i]))
		builder.WriteString(" | ")
		builder.WriteString(renderHistogramBar(bucket.Count, layout.maxCount))
		builder.WriteString(" (")
		builder.WriteString(fmt.Sprintf("%-*s", layout.countWidth, layout.counts[i]))
		builder.WriteString(")")
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%*s", layout.percentWidth, layout.percents[i]))
		builder.WriteString("\n")
	}

	return builder.String()
}

type histogramRenderLayout struct {
	from         []string
	to           []string
	counts       []string
	percents     []string
	fromWidth    int
	toWidth      int
	countWidth   int
	percentWidth int
	maxCount     int
	totalCount   int
}

func buildHistogramRenderLayout(bins []analysis.HistogramBucket) histogramRenderLayout {
	hl := histogramRenderLayout{
		from:     make([]string, len(bins)),
		to:       make([]string, len(bins)),
		counts:   make([]string, len(bins)),
		percents: make([]string, len(bins)),
	}

	for i, bucket := range bins {
		hl.from[i] = bucket.From.String()
		hl.to[i] = bucket.To.String()
		hl.counts[i] = fmt.Sprintf("%d", bucket.Count)
		hl.totalCount += bucket.Count

		if bucket.Count > hl.maxCount {
			hl.maxCount = bucket.Count
		}
		hl.fromWidth = max(hl.fromWidth, len(hl.from[i]))
		hl.toWidth = max(hl.toWidth, len(hl.to[i]))
		hl.countWidth = max(hl.countWidth, len(hl.counts[i]))
	}

	if hl.maxCount == 0 {
		hl.maxCount = 1
	}
	if hl.totalCount == 0 {
		hl.totalCount = 1
	}

	for i, bucket := range bins {
		hl.percents[i] = formatHistogramPercent(bucket.Count, hl.totalCount)
		hl.percentWidth = max(hl.percentWidth, len(hl.percents[i]))
	}
	return hl
}

func renderHistogramBar(count, maxCount int) string {
	if maxCount < 1 {
		maxCount = 1
	}
	if count < 0 {
		count = 0
	}
	fill := 0
	if count > 0 {
		fill = int(math.Round(float64(count) / float64(maxCount) * float64(histogramBarWidth)))
		if fill == 0 {
			fill = 1
		}
	}
	if fill > histogramBarWidth {
		fill = histogramBarWidth
	}
	empty := histogramBarWidth - fill
	if empty < 0 {
		empty = 0
	}
	return strings.Repeat("█", fill) + strings.Repeat("░", empty)
}

func formatHistogramPercent(count, total int) string {
	if total <= 0 {
		return "0%"
	}
	percent := (float64(count) / float64(total)) * 100
	return fmt.Sprintf("%.1f%%", percent)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func renderHistogramLegend(indent string) string {
	if indent == "" {
		indent = histogramDefaultIndent
	}
	entryIndent := indent + histogramDefaultIndent
	lines := []string{
		fmt.Sprintf("%sLegend:", indent),
		fmt.Sprintf("%sgreen <= p50", entryIndent),
		fmt.Sprintf("%syellow between p50–p90", entryIndent),
		fmt.Sprintf(
			"%sred overlaps or exceeds p90 (faded when bucket <%d%% of busiest)",
			entryIndent,
			histogramFadePercent,
		),
	}
	return strings.Join(lines, "\n") + "\n"
}
