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

	maxBarCount := 0
	maxFromWidth := 0
	maxToWidth := 0
	maxCountWidth := 0
	totalCount := 0
	maxPercentWidth := 0

	formattedFrom := make([]string, len(bins))
	formattedTo := make([]string, len(bins))
	counts := make([]string, len(bins))
	percents := make([]string, len(bins))

	for i, bucket := range bins {
		formattedFrom[i] = bucket.From.String()
		formattedTo[i] = bucket.To.String()
		counts[i] = fmt.Sprintf("%d", bucket.Count)
		totalCount += bucket.Count

		if bucket.Count > maxBarCount {
			maxBarCount = bucket.Count
		}
		if w := len(formattedFrom[i]); w > maxFromWidth {
			maxFromWidth = w
		}
		if w := len(formattedTo[i]); w > maxToWidth {
			maxToWidth = w
		}
		if w := len(counts[i]); w > maxCountWidth {
			maxCountWidth = w
		}
	}

	if maxBarCount == 0 {
		maxBarCount = 1
	}
	if totalCount == 0 {
		totalCount = 1
	}

	for i, bucket := range bins {
		percent := int(math.Round(float64(bucket.Count) / float64(totalCount) * 100))
		percents[i] = fmt.Sprintf("%d%%", percent)
		if w := len(percents[i]); w > maxPercentWidth {
			maxPercentWidth = w
		}
		if w := len(formattedFrom[i]); w > maxFromWidth {
			maxFromWidth = w
		}
		if w := len(formattedTo[i]); w > maxToWidth {
			maxToWidth = w
		}
	}

	rowIndent := indent + "  "
	var builder strings.Builder

	for i, bucket := range bins {
		barLen := int((float64(bucket.Count) / float64(maxBarCount)) * float64(histogramBarWidth))
		if barLen < 0 {
			barLen = 0
		}
		bar := strings.Repeat("#", barLen)
		builder.WriteString(rowIndent)
		builder.WriteString(fmt.Sprintf("%-*s", maxFromWidth, formattedFrom[i]))
		builder.WriteString(" – ")
		builder.WriteString(fmt.Sprintf("%-*s", maxToWidth, formattedTo[i]))
		builder.WriteString(" | ")
		if barLen < histogramBarWidth {
			builder.WriteString(fmt.Sprintf("%-*s", histogramBarWidth, bar))
		} else {
			builder.WriteString(bar)
		}
		builder.WriteString(" (")
		builder.WriteString(fmt.Sprintf("%-*s", maxCountWidth, counts[i]))
		builder.WriteString(")")
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%*s", maxPercentWidth, percents[i]))
		builder.WriteString("\n")
	}

	return builder.String()
}

func renderHistogramLegend(indent string) string {
	if indent == "" {
		indent = histogramDefaultIndent
	}
	entryIndent := indent + "  "
	lines := []string{
		fmt.Sprintf("%sLegend:", indent),
		fmt.Sprintf("%sgreen <= p50", entryIndent),
		fmt.Sprintf("%syellow between p50–p90", entryIndent),
		fmt.Sprintf("%sred overlaps or exceeds p90 (faded when bucket <%d%% of busiest)", entryIndent, histogramFadePercent),
	}
	return strings.Join(lines, "\n") + "\n"
}
