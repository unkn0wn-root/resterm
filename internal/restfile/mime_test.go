package restfile

import "testing"

func TestIsMultipartMime(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"multipart/form-data; boundary=x", true},
		{"multipart/mixed", true},
		{"MULTIPART/Related", true},
		{"  multipart/form-data", true},
		{"application/json", false},
		{"{{ct}}", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsMultipartMime(tt.ct); got != tt.want {
			t.Errorf("IsMultipartMime(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}
