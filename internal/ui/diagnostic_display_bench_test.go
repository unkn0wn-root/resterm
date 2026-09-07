package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

// Measure the entire display refresh input path, including Value when needed.
// Parsing and editor setup are excluded: neither belongs to display filtering.
func BenchmarkDiagnosticDisplayRefresh(b *testing.B) {
	for _, tt := range []struct {
		name           string
		lines, first   int
		stride, suffix int
	}{
		{"small", 50, 2, 0, 0},
		{"large clean", 10000, -1, 0, 0},
		{"large early", 10000, 2, 0, 0},
		{"large late", 10000, 9998, 0, 0},
		{"large sparse", 10000, 2, 1000, 0},
		{"large dense", 10000, 0, 1, 0},
		{"long unicode", 10, 2, 0, 32000},
	} {
		b.Run(tt.name, func(b *testing.B) {
			lines := make([]string, tt.lines)
			for i := range lines {
				lines[i] = "# ordinary comment in a request file"
			}
			var items []diag.Diagnostic
			for i := tt.first; i >= 0 && i < len(lines); i += tt.stride {
				lines[i] = "# @mock path=/x" + strings.Repeat("漢", tt.suffix)
				items = append(items, diag.Diagnostic{
					Severity: diag.SeverityError,
					Span:     diag.Span{Start: diag.Pos{Line: i + 1, Col: 3}, End: diag.Pos{Line: i + 1, Col: 8}},
				})
				if tt.stride == 0 {
					break
				}
			}
			source := strings.Join(lines, "\n")
			display := newDiagnosticSnapshot(diagnosticKey{}, diag.Report{Source: []byte(source), Items: items}).display
			editor := textarea.New()
			editor.SetValue(source + "x")
			b.ReportAllocs()
			for b.Loop() {
				display.afterEdit(&editor)
			}
		})
	}
}
