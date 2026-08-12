package delay

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

func draws(t *testing.T, s Spec, n int) []time.Duration {
	t.Helper()
	r := rand.New(rand.NewPCG(1, 2))
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = s.draw(r)
	}
	return out
}

func mustParse(t *testing.T, raw string) Spec {
	t.Helper()
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	return s
}

func TestParseText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{input: "150ms", want: "150ms"},
		{input: " 2s ", want: "2s"},
		{input: "1h30m", want: "1h30m0s"},
		{input: "0", want: ""},
		{input: "random(100ms,500ms)", want: "random(100ms,500ms)"},
		{input: "random(100ms, 500ms)", want: "random(100ms,500ms)"},
		{input: "RANDOM(1s,1s)", want: "random(1s,1s)"},
		{input: "normal(250ms,50ms)", want: "normal(250ms,50ms)"},
		{input: "normal(250ms,20%)", want: "normal(250ms,20%)"},
		{input: "jitter(200ms,20%)", want: "jitter(200ms,20%)"},
		{input: "jitter(200ms,12.5%)", want: "jitter(200ms,12.5%)"},
		{input: "jitter(200ms,20ms)", want: "jitter(200ms,20ms)"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			s := mustParse(t, tc.input)
			if got := s.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
			if tc.want == "" {
				return
			}
			if again := mustParse(t, s.String()); again != s {
				t.Fatalf("reparsed %#v, want %#v", again, s)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{input: "", want: "must be a duration"},
		{input: "soon", want: "must be a duration"},
		{input: "-5ms", want: "cannot be negative"},
		{input: "gauss(1ms,2ms)", want: `"gauss" is not a delay distribution`},
		{input: "random(100ms,500ms", want: `missing a closing ")"`},
		{input: "random(100ms)", want: "random takes 2 arguments, got 1"},
		{input: "random(1ms,2ms,3ms)", want: "random takes 2 arguments, got 3"},
		{input: "random()", want: "random takes 2 arguments, got 0"},
		{input: "random(500ms,100ms)", want: "random max cannot be shorter than min"},
		{input: "random(100ms,20%)", want: "random max must be a duration, not a percentage"},
		{input: "random(soon,1s)", want: `random min "soon" is not a duration`},
		{input: "random(-1s,1s)", want: "random min cannot be negative"},
		{input: "normal(250ms,-10ms)", want: "normal stddev cannot be negative"},
		{input: "normal(250ms,-5%)", want: "normal stddev cannot be negative"},
		{input: "normal(250ms,NaN%)", want: `normal stddev "NaN%" is not a percentage`},
		{input: "jitter(200ms,soon)", want: `jitter spread "soon" is not a duration`},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			s, err := Parse(tc.input)
			if err == nil {
				t.Fatalf("Parse(%q) = %#v, want an error", tc.input, s)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestSampleBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		min, max time.Duration
	}{
		{input: "150ms", min: 150 * time.Millisecond, max: 150 * time.Millisecond},
		{input: "random(100ms,500ms)", min: 100 * time.Millisecond, max: 500 * time.Millisecond},
		{input: "jitter(200ms,20%)", min: 160 * time.Millisecond, max: 240 * time.Millisecond},
		{input: "jitter(200ms,50ms)", min: 150 * time.Millisecond, max: 250 * time.Millisecond},
		// A spread wider than the base only reaches down to no delay at all.
		{input: "jitter(100ms,300%)", min: 0, max: 400 * time.Millisecond},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			for _, got := range draws(t, mustParse(t, tc.input), 2000) {
				if got < tc.min || got > tc.max {
					t.Fatalf("sample = %s, want within [%s, %s]", got, tc.min, tc.max)
				}
			}
		})
	}
}

func TestSampleNormalSpreadsAroundMean(t *testing.T) {
	t.Parallel()

	const n = 20000
	samples := draws(t, mustParse(t, "normal(250ms,50ms)"), n)
	var total time.Duration
	tail := 0
	for _, got := range samples {
		if got < 0 {
			t.Fatalf("sample = %s, want a non-negative wait", got)
		}
		total += got
		if got < 200*time.Millisecond || got > 300*time.Millisecond {
			tail++
		}
	}

	if mean := total / n; mean < 245*time.Millisecond || mean > 255*time.Millisecond {
		t.Fatalf("mean = %s, want it near 250ms", mean)
	}
	// About a third of a normal distribution sits beyond one deviation.
	if tail < n/5 || tail > n/2 {
		t.Fatalf("%d of %d samples fell outside one deviation", tail, n)
	}
}

func TestSampleNormalNeverNegative(t *testing.T) {
	t.Parallel()

	for _, got := range draws(t, mustParse(t, "normal(10ms,50ms)"), 5000) {
		if got < 0 {
			t.Fatalf("sample = %s, want a non-negative wait", got)
		}
	}
}

func TestRelativeDeviationResolvesAgainstBase(t *testing.T) {
	t.Parallel()

	if got := mustParse(t, "normal(250ms,20%)").deviation(); got != 50*time.Millisecond {
		t.Fatalf("deviation() = %s, want 50ms", got)
	}
	if got := mustParse(t, "jitter(1s,0.5%)").deviation(); got != 5*time.Millisecond {
		t.Fatalf("deviation() = %s, want 5ms", got)
	}
}

func TestFixed(t *testing.T) {
	t.Parallel()

	s := Fixed(150 * time.Millisecond)
	if got := s.Sample(); got != 150*time.Millisecond {
		t.Fatalf("Sample() = %s, want 150ms", got)
	}
	if s.IsZero() || s.String() != "150ms" {
		t.Fatalf("unexpected spec %#v", s)
	}

	var zero Spec
	if !zero.IsZero() || zero.String() != "" || zero.Sample() != 0 {
		t.Fatalf("unexpected zero spec %#v", zero)
	}
	if got := Fixed(-time.Second).Sample(); got != 0 {
		t.Fatalf("Sample() = %s, want no wait", got)
	}
}

func TestMarshalText(t *testing.T) {
	t.Parallel()

	type holder struct {
		Latency Spec
	}
	data, err := json.Marshal(holder{Latency: mustParse(t, "jitter(200ms,20%)")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(data), `{"Latency":"jitter(200ms,20%)"}`; got != want {
		t.Fatalf("marshaled %s, want %s", got, want)
	}

	var back holder
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Latency != mustParse(t, "jitter(200ms,20%)") {
		t.Fatalf("decoded %#v", back.Latency)
	}

	var empty holder
	if err := json.Unmarshal([]byte(`{"Latency":""}`), &empty); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if !empty.Latency.IsZero() {
		t.Fatalf("decoded %#v, want the zero spec", empty.Latency)
	}
	if err := json.Unmarshal([]byte(`{"Latency":"gauss(1s,1s)"}`), &empty); err == nil {
		t.Fatal("expected an error for an unknown distribution")
	}
}

func TestDistributions(t *testing.T) {
	t.Parallel()

	forms := Distributions()
	if len(forms) == 0 {
		t.Fatal("no distributions described")
	}
	for _, f := range forms {
		if f.Name() == "" || f.Summary() == "" {
			t.Fatalf("incomplete descriptor %+v", f)
		}
		s := mustParse(t, f.Usage())
		if s.String() != f.Usage() {
			t.Fatalf("usage %q parsed as %q", f.Usage(), s)
		}
		if !strings.Contains(formsHint, f.Usage()) {
			t.Fatalf("hint %q omits %q", formsHint, f.Usage())
		}
	}
}
