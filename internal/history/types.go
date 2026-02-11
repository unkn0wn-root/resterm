package history

import "time"

type Entry struct {
	ID             string          `json:"id"`
	ExecutedAt     time.Time       `json:"executedAt"`
	Environment    string          `json:"environment"`
	RequestName    string          `json:"requestName"`
	FilePath       string          `json:"filePath"`
	Method         string          `json:"method"`
	URL            string          `json:"url"`
	Status         string          `json:"status"`
	StatusCode     int             `json:"statusCode"`
	Duration       time.Duration   `json:"duration"`
	BodySnippet    string          `json:"bodySnippet"`
	RequestText    string          `json:"requestText"`
	Description    string          `json:"description,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	ProfileResults *ProfileResults `json:"profileResults,omitempty"`
	Trace          *TraceSummary   `json:"trace,omitempty"`
	Compare        *CompareEntry   `json:"compare,omitempty"`
}

type CompareEntry struct {
	Baseline string          `json:"baseline"`
	Results  []CompareResult `json:"results"`
}

type CompareResult struct {
	Environment string        `json:"environment"`
	Status      string        `json:"status"`
	StatusCode  int           `json:"statusCode"`
	Duration    time.Duration `json:"duration"`
	BodySnippet string        `json:"bodySnippet"`
	RequestText string        `json:"requestText"`
	Error       string        `json:"error,omitempty"`
}

type ProfileResults struct {
	TotalRuns      int                   `json:"totalRuns"`
	WarmupRuns     int                   `json:"warmupRuns"`
	SuccessfulRuns int                   `json:"successfulRuns"`
	FailedRuns     int                   `json:"failedRuns"`
	Latency        *ProfileLatency       `json:"latency,omitempty"`
	Percentiles    []ProfilePercentile   `json:"percentiles,omitempty"`
	Histogram      []ProfileHistogramBin `json:"histogram,omitempty"`
}

type ProfileLatency struct {
	Count  int           `json:"count"`
	Min    time.Duration `json:"min"`
	Max    time.Duration `json:"max"`
	Mean   time.Duration `json:"mean"`
	Median time.Duration `json:"median"`
	StdDev time.Duration `json:"stdDev"`
}

type ProfilePercentile struct {
	Percentile int           `json:"percentile"`
	Value      time.Duration `json:"value"`
}

type ProfileHistogramBin struct {
	From  time.Duration `json:"from"`
	To    time.Duration `json:"to"`
	Count int           `json:"count"`
}
