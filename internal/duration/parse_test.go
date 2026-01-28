package duration

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "zero", input: "0", want: 0},
		{name: "hours", input: "1h", want: time.Hour},
		{name: "mixed", input: "1h30m", want: time.Hour + 30*time.Minute},
		{name: "days", input: "2d", want: 48 * time.Hour},
		{
			name:  "weeks",
			input: "1w2d3h4m5s",
			want:  9*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second,
		},
		{name: "decimal", input: "1.5d", want: 36 * time.Hour},
		{name: "spaces", input: "1h 30m", want: time.Hour + 30*time.Minute},
		{name: "negative", input: "-6d", want: -6 * 24 * time.Hour},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Parse(tc.input)
			if !ok {
				t.Fatalf("expected ok for %q", tc.input)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"",
		"   ",
		"d",
		"1x",
		"1d2",
		"1.2.3s",
		"1h-30m",
	}

	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, ok := Parse(input); ok {
				t.Fatalf("expected invalid for %q", input)
			}
		})
	}
}
