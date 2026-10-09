package headless

import (
	"bytes"
	"encoding/json"
)

// MarshalJSON writes the canonical report JSON format.
func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.model())
}

// UnmarshalJSON reads the report JSON format. Durations come back in whole milliseconds.
func (r *Report) UnmarshalJSON(b []byte) error {
	return unmarshal(b, r, newReport)
}

// MarshalJSON writes the canonical result JSON format.
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.model())
}

// UnmarshalJSON reads the result JSON format.
func (r *Result) UnmarshalJSON(b []byte) error {
	return unmarshal(b, r, newResult)
}

// MarshalJSON writes the canonical step JSON format.
func (s Step) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.model())
}

// UnmarshalJSON reads the step JSON format.
func (s *Step) UnmarshalJSON(b []byte) error {
	return unmarshal(b, s, newStep)
}

// MarshalJSON writes the failure the way it appears in the report JSON.
func (f Failure) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.model())
}

// UnmarshalJSON reads the failure the way it appears in the report JSON.
func (f *Failure) UnmarshalJSON(b []byte) error {
	return unmarshal(b, f, newFailure)
}

// MarshalJSON writes the test the way it appears in the report JSON.
func (t Test) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// UnmarshalJSON reads the test the way it appears in the report JSON.
func (t *Test) UnmarshalJSON(b []byte) error {
	return unmarshal(b, t, newTest)
}

// MarshalJSON writes the profile the way it appears in the report JSON.
func (p Profile) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// UnmarshalJSON reads the profile the way it appears in the report JSON.
func (p *Profile) UnmarshalJSON(b []byte) error {
	return unmarshal(b, p, newProfile)
}

// MarshalJSON writes the profile failure the way it appears in the report JSON.
func (p ProfileFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// UnmarshalJSON reads the profile failure the way it appears in the report JSON.
func (p *ProfileFailure) UnmarshalJSON(b []byte) error {
	return unmarshal(b, p, newProfileFailure)
}

// MarshalJSON writes the latency the way it appears in the report JSON.
func (l Latency) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.model())
}

// UnmarshalJSON reads the latency the way it appears in the report JSON.
func (l *Latency) UnmarshalJSON(b []byte) error {
	return unmarshal(b, l, newLatency)
}

// MarshalJSON writes the percentile the way it appears in the report JSON.
func (p Percentile) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// UnmarshalJSON reads the percentile the way it appears in the report JSON.
func (p *Percentile) UnmarshalJSON(b []byte) error {
	return unmarshal(b, p, newPercentile)
}

// MarshalJSON writes the bin the way it appears in the report JSON.
func (h HistBin) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.model())
}

// UnmarshalJSON reads the bin the way it appears in the report JSON.
func (h *HistBin) UnmarshalJSON(b []byte) error {
	return unmarshal(b, h, newHistBin)
}

// MarshalJSON writes the stream the way it appears in the report JSON.
func (s Stream) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.model())
}

// UnmarshalJSON reads the stream the way it appears in the report JSON.
func (s *Stream) UnmarshalJSON(b []byte) error {
	return unmarshal(b, s, newStream)
}

// MarshalJSON writes the trace the way it appears in the report JSON.
func (t Trace) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// UnmarshalJSON reads the trace the way it appears in the report JSON.
func (t *Trace) UnmarshalJSON(b []byte) error {
	return unmarshal(b, t, newTrace)
}

// MarshalJSON writes the budget the way it appears in the report JSON.
func (t TraceBudget) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// UnmarshalJSON reads the budget the way it appears in the report JSON.
func (t *TraceBudget) UnmarshalJSON(b []byte) error {
	return unmarshal(b, t, newTraceBudget)
}

// MarshalJSON writes the breach the way it appears in the report JSON.
func (t TraceBreach) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// UnmarshalJSON reads the breach the way it appears in the report JSON.
func (t *TraceBreach) UnmarshalJSON(b []byte) error {
	return unmarshal(b, t, newTraceBreach)
}

func unmarshal[M, P any](b []byte, dst *P, conv func(M) P) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	var m M
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*dst = conv(m)
	return nil
}
