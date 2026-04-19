package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/runfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestWriteTextStyledAppliesANSIColor(t *testing.T) {
	rep := &runfmt.Report{
		FilePath: "workflow.http",
		EnvName:  "dev",
		Results: []runfmt.Result{{
			Kind:     "workflow",
			Name:     "sample-order",
			Method:   "WORKFLOW",
			Status:   runfmt.StatusPass,
			Duration: 125 * time.Millisecond,
			Steps: []runfmt.Step{
				{
					Name:     "Login",
					Status:   runfmt.StatusPass,
					Duration: 25 * time.Millisecond,
					HTTP:     &runfmt.HTTP{Status: "200 OK", StatusCode: 200},
				},
				{
					Name:     "Checkout",
					Status:   runfmt.StatusFail,
					Summary:  "unexpected status code 500",
					Duration: 50 * time.Millisecond,
				},
			},
		}},
		Total:   1,
		Passed:  0,
		Failed:  1,
		Skipped: 0,
	}

	var plain strings.Builder
	if err := runfmt.WriteText(&plain, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	var colored strings.Builder
	if err := WriteTextStyled(&colored, rep, termcolor.TrueColor()); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	out := colored.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ansi output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain.String() {
		t.Fatalf(
			"expected stripped output to match plain text\nwant:\n%s\n\ngot:\n%s",
			plain.String(),
			got,
		)
	}
}
