package ui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestStatusBarHexRGB(t *testing.T) {
	tests := []struct {
		in      lipgloss.Color
		r, g, b int
		ok      bool
	}{
		{in: "#14b8a6", r: 20, g: 184, b: 166, ok: true},
		{in: "#FFD46A", r: 255, g: 212, b: 106, ok: true}, // uppercase
		{in: "#abc", r: 170, g: 187, b: 204, ok: true},    // shorthand expands
		{in: "230", ok: false},                            // ANSI index, not #230 shorthand
		{in: "5", ok: false},                              // ANSI index
		{in: "red", ok: false},                            // named color
		{in: "#12xy56", ok: false},                        // non-hex digits
		{in: "#", ok: false},                              // empty body
		{in: "", ok: false},                               // empty
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", string(tt.in)), func(t *testing.T) {
			r, g, b, ok := statusBarHexRGB(tt.in)
			if ok != tt.ok {
				t.Fatalf("statusBarHexRGB(%q) ok=%v, want %v", tt.in, ok, tt.ok)
			}
			if ok && (r != tt.r || g != tt.g || b != tt.b) {
				t.Fatalf("statusBarHexRGB(%q)=(%d,%d,%d), want (%d,%d,%d)",
					tt.in, r, g, b, tt.r, tt.g, tt.b)
			}
		})
	}
}
