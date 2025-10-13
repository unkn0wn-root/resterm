package ui

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

func renderHistogram(bins []analysis.HistogramBucket) string {
	if len(bins) == 0 {
		return ""
	}

	maxBarCount := 0
	maxFromWidth := 0
	maxToWidth := 0
	maxCountWidth := 0

	formattedFrom := make([]string, len(bins))
	formattedTo := make([]string, len(bins))
	counts := make([]string, len(bins))

	for i, bucket := range bins {
		formattedFrom[i] = bucket.From.String()
		formattedTo[i] = bucket.To.String()
		counts[i] = fmt.Sprintf("%d", bucket.Count)

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

	const barWidth = 20
	var builder strings.Builder
	builder.WriteString("  Histogram:\n")

	for i, bucket := range bins {
		barLen := int((float64(bucket.Count) / float64(maxBarCount)) * float64(barWidth))
		if barLen < 0 {
			barLen = 0
		}
		bar := strings.Repeat("#", barLen)
		builder.WriteString("    ")
		builder.WriteString(fmt.Sprintf("%-*s", maxFromWidth, formattedFrom[i]))
		builder.WriteString(" – ")
		builder.WriteString(fmt.Sprintf("%-*s", maxToWidth, formattedTo[i]))
		builder.WriteString(" | ")
		if barLen < barWidth {
			builder.WriteString(fmt.Sprintf("%-*s", barWidth, bar))
		} else {
			builder.WriteString(bar)
		}
		builder.WriteString(" (")
		builder.WriteString(fmt.Sprintf("%-*s", maxCountWidth, counts[i]))
		builder.WriteString(")\n")
	}

	return builder.String()
}
