package bytesize

import (
	"math"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "zero", raw: "0", want: 0},
		{name: "bytes without suffix", raw: "42", want: 42},
		{name: "bytes suffix", raw: "42 B", want: 42},
		{name: "kilobytes", raw: "1kb", want: 1 << 10},
		{name: "kibibytes", raw: "1 KiB", want: 1 << 10},
		{name: "megabytes", raw: "1MB", want: 1 << 20},
		{name: "mebibytes", raw: "1 mib", want: 1 << 20},
		{name: "gigabytes", raw: "1GB", want: 1 << 30},
		{name: "gibibytes", raw: "1 GiB", want: 1 << 30},
		{name: "surrounding whitespace", raw: "\t 2 KiB \n", want: 2 << 10},
		{name: "leading plus", raw: "+2 KiB", want: 2 << 10},
		{name: "leading decimal point", raw: ".5 KiB", want: 512},
		{name: "trailing decimal point", raw: "1. KiB", want: 1 << 10},
		{name: "fractional unit", raw: "1.5 KiB", want: 1536},
		{name: "one byte as kibibytes", raw: "0.0009765625 KiB", want: 1},
		{
			name: "one byte as gibibytes",
			raw:  "0.000000000931322574615478515625 GiB",
			want: 1,
		},
		{
			name: "maximum int64",
			raw:  "9223372036854775807",
			want: math.MaxInt64,
		},
		{
			name: "maximum int64 as kibibytes",
			raw:  "9007199254740991.9990234375 KiB",
			want: math.MaxInt64,
		},
		{
			name: "integer beyond exact float64 range",
			raw:  "9007199254740993",
			want: 9007199254740993,
		},
		{
			name: "redundant fractional zeroes",
			raw:  "1.0000000000000000000000000000000000000000 B",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse(%q) returned unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "whitespace", raw: " \t\n"},
		{name: "negative", raw: "-1"},
		{name: "negative zero", raw: "-0"},
		{name: "missing number", raw: "KiB"},
		{name: "sign only", raw: "+"},
		{name: "decimal point only", raw: "."},
		{name: "multiple decimal points", raw: "1.2.3 KiB"},
		{name: "exponent", raw: "1e3"},
		{name: "hexadecimal float", raw: "0x1p10"},
		{name: "digit separator", raw: "1_024"},
		{name: "not a number", raw: "NaN"},
		{name: "infinity", raw: "Inf"},
		{name: "unknown suffix", raw: "1 TB"},
		{name: "suffix containing digits", raw: "1 KiB2"},
		{name: "whitespace within suffix", raw: "1 K iB"},
		{name: "fractional byte", raw: "0.5 B"},
		{name: "fractional kibibyte", raw: "0.1 KiB"},
		{name: "int64 overflow", raw: "9223372036854775808"},
		{name: "int64 overflow with suffix", raw: "9007199254740992 KiB"},
		{
			name: "precision cannot resolve to whole byte",
			raw:  "0.0000000000000000000000000000001 GiB",
		},
		{name: "very large number", raw: strings.Repeat("9", 256)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got, err := Parse(tt.raw); err == nil {
				t.Fatalf("Parse(%q) = %d, want error", tt.raw, got)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"0",
		"1.5 KiB",
		"9223372036854775807",
		"9223372036854775808",
		"NaN",
		"Inf",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got, err := Parse(raw)
		if err == nil && got < 0 {
			t.Fatalf("Parse(%q) = %d without an error", raw, got)
		}
	})
}
