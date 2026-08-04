package bodyfmt

import "testing"

func TestStripANSIUsesLegacyUIBehavior(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "csi",
			in:   "\x1b[31mred\x1b[0m",
			want: "red",
		},
		{
			name: "osc",
			in:   "\x1b]8;;https://example.com\x07label\x1b]8;;\x07",
			want: "label",
		},
		{
			name: "incomplete csi preserved",
			in:   "\x1b[31",
			want: "\x1b[31",
		},
		{
			name: "non csi escape preserved",
			in:   "\x1bc",
			want: "\x1bc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripANSI(tt.in); got != tt.want {
				t.Fatalf("StripANSI(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsEmptyIgnoresANSI(t *testing.T) {
	if !IsEmpty("\x1b[31m \x1b[0m") {
		t.Fatalf("expected ANSI-only string to count as empty")
	}
	if IsEmpty("\x1b[31mx\x1b[0m") {
		t.Fatalf("expected string with text to count as non-empty")
	}
}

func TestJoinSectionsDropsBlankSections(t *testing.T) {
	got := JoinSections("first\n", "", "   ", "\r\nsecond\r\n")
	want := "first\n\nsecond"
	if got != want {
		t.Fatalf("JoinSections()=%q, want %q", got, want)
	}
}
