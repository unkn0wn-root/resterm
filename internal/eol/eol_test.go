package eol

import (
	"bufio"
	"strings"
	"testing"
)

func TestCut(t *testing.T) {
	tests := []struct {
		in   string
		text string
		term string
	}{
		{in: "a\r\n", text: "a", term: CRLF},
		{in: "a\n", text: "a", term: LF},
		{in: "a", text: "a"},
		{in: "\r\n", term: CRLF},
		{in: "a\r", text: "a\r"},
		{in: ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			text, term := Cut(tt.in)
			if text != tt.text || term != tt.term {
				t.Fatalf("Cut(%q) = (%q, %q), want (%q, %q)", tt.in, text, term, tt.text, tt.term)
			}
		})
	}
}

func TestLinesRoundTrips(t *testing.T) {
	for _, in := range []string{
		"",
		"a",
		"a\n",
		"a\r\nb",
		"a\r\nb\r\n",
		"a\n\r\nb",
		"\n\n",
	} {
		t.Run(in, func(t *testing.T) {
			var b strings.Builder
			for text, term := range Lines(in) {
				b.WriteString(text)
				b.WriteString(term)
			}
			if b.String() != in {
				t.Fatalf("rebuilt %q, want %q", b.String(), in)
			}
		})
	}
}

func TestLinesReportsTerminators(t *testing.T) {
	var got []string
	for text, term := range Lines("a\r\nb\nc") {
		got = append(got, text+"|"+term)
	}
	want := []string{"a|\r\n", "b|\n", "c|"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Lines = %v, want %v", got, want)
	}
}

func TestLinesStopsEarly(t *testing.T) {
	seen := 0
	for range Lines("a\nb\nc\n") {
		seen++
		break
	}
	if seen != 1 {
		t.Fatalf("saw %d lines, want 1", seen)
	}
}

func TestScanLinesKeepsTerminators(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("a\r\nb\nc"))
	scanner.Split(ScanLines)

	var got []string
	for scanner.Scan() {
		got = append(got, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"a\r\n", "b\n", "c"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ScanLines = %q, want %q", got, want)
	}
}
