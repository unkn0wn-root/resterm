package bodyfmt

import "testing"

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{in: -1, want: "0 B"},
		{in: 0, want: "0 B"},
		{in: 1023, want: "1023 B"},
		{in: 1024, want: "1 KiB"},
		{in: 1536, want: "1.5 KiB"},
		{in: 1024 * 1024, want: "1 MiB"},
		{in: 5 * 1024 * 1024 * 1024, want: "5 GiB"},
	}

	for _, tt := range tests {
		if got := FormatByteSize(tt.in); got != tt.want {
			t.Fatalf("FormatByteSize(%d)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatByteQuantity(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{in: 0, want: "0 bytes"},
		{in: 1, want: "1 byte"},
		{in: 2, want: "2 bytes"},
	}

	for _, tt := range tests {
		if got := FormatByteQuantity(tt.in); got != tt.want {
			t.Fatalf("FormatByteQuantity(%d)=%q, want %q", tt.in, got, tt.want)
		}
	}
}
