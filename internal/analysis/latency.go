package analysis

import (
	"math"
	"sort"
	"time"
)

type HistogramBucket struct {
	From  time.Duration
	To    time.Duration
	Count int
}

type LatencyStats struct {
	Count       int
	Min         time.Duration
	Max         time.Duration
	Mean        time.Duration
	Median      time.Duration
	StdDev      time.Duration
	Percentiles map[int]time.Duration
	Histogram   []HistogramBucket
}

func ComputeLatencyStats(durations []time.Duration, percentiles []int, bins int) LatencyStats {
	stats := LatencyStats{}
	count := len(durations)
	if count == 0 {
		return stats
	}

	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats.Count = count
	stats.Min = sorted[0]
	stats.Max = sorted[count-1]

	sum := time.Duration(0)
	for _, d := range sorted {
		sum += d
	}
	stats.Mean = sum / time.Duration(count)

	if count%2 == 0 {
		mid := count / 2
		stats.Median = (sorted[mid-1] + sorted[mid]) / 2
	} else {
		stats.Median = sorted[count/2]
	}

	stats.StdDev = computeStdDev(sorted, stats.Mean)

	if len(percentiles) > 0 {
		stats.Percentiles = computePercentiles(sorted, percentiles)
	}

	if bins <= 0 {
		bins = 10
	}
	stats.Histogram = buildHistogram(sorted, bins)

	return stats
}

func computeStdDev(values []time.Duration, mean time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}

	meanMS := float64(mean) / float64(time.Millisecond)
	var sumSquares float64
	for _, d := range values {
		ms := float64(d) / float64(time.Millisecond)
		delta := ms - meanMS
		sumSquares += delta * delta
	}

	variance := sumSquares / float64(len(values))
	sdMS := math.Sqrt(variance)
	return time.Duration(sdMS * float64(time.Millisecond))
}

func computePercentiles(values []time.Duration, percentiles []int) map[int]time.Duration {
	result := make(map[int]time.Duration, len(percentiles))
	sortedPerc := append([]int(nil), percentiles...)
	sort.Ints(sortedPerc)
	count := len(values)

	for _, p := range sortedPerc {
		if p <= 0 {
			result[p] = values[0]
			continue
		}
		if p >= 100 {
			result[p] = values[count-1]
			continue
		}

		rank := float64(p) / 100 * float64(count)
		idx := int(math.Ceil(rank)) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= count {
			idx = count - 1
		}
		result[p] = values[idx]
	}

	return result
}

func buildHistogram(values []time.Duration, bins int) []HistogramBucket {
	if len(values) == 0 {
		return nil
	}

	if bins > len(values) {
		bins = len(values)
	}

	min := float64(values[0]) / float64(time.Millisecond)
	max := float64(values[len(values)-1]) / float64(time.Millisecond)
	delta := max - min
	if delta <= 0 {
		return []HistogramBucket{{From: values[0], To: values[len(values)-1], Count: len(values)}}
	}

	width := delta / float64(bins)
	if width <= 0 {
		width = delta / float64(bins)
	}

	counts := make([]int, bins)
	for _, d := range values {
		v := float64(d) / float64(time.Millisecond)
		bucket := int(math.Floor((v - min) / width))
		if bucket >= bins {
			bucket = bins - 1
		}
		if bucket < 0 {
			bucket = 0
		}
		counts[bucket]++
	}

	hist := make([]HistogramBucket, bins)
	for i := 0; i < bins; i++ {
		from := min + float64(i)*width
		to := from + width
		if i == bins-1 {
			to = max
		}
		hist[i] = HistogramBucket{
			From:  durationFromMillis(from),
			To:    durationFromMillis(to),
			Count: counts[i],
		}
	}

	return hist
}

func durationFromMillis(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
