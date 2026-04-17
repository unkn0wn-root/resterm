package runfmt

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestWriteTextStyledColorPreservesPlainText(t *testing.T) {
	rep := &Report{
		FilePath: "workflow.http",
		EnvName:  "dev",
		Results: []Result{{
			Kind:     "workflow",
			Name:     "sample-order",
			Method:   "WORKFLOW",
			Status:   StatusPass,
			Duration: 125 * time.Millisecond,
			Steps: []Step{
				{
					Name:     "Login",
					Status:   StatusPass,
					Duration: 25 * time.Millisecond,
					HTTP:     &HTTP{Status: "200 OK", StatusCode: 200},
				},
				{
					Name:     "Checkout",
					Status:   StatusFail,
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
	if err := WriteText(&plain, rep); err != nil {
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

func TestWriteTextIncludesTargetDetailsWhenDifferent(t *testing.T) {
	rep := &Report{
		FilePath: "reports.http",
		Results: []Result{{
			Kind:            "request",
			Name:            "reports",
			Method:          "GET",
			Target:          "{{services.api.base}}/reports",
			EffectiveTarget: "https://httpbin.org/anything/api/reports",
			Status:          StatusPass,
			Duration:        463 * time.Millisecond,
			HTTP:            &HTTP{Status: "200 OK", StatusCode: 200},
			Steps: []Step{{
				Name:            "dev",
				Method:          "GET",
				Target:          "{{services.api.base}}/reports",
				EffectiveTarget: "https://dev.httpbin.org/anything/api/reports",
				Status:          StatusPass,
				Duration:        250 * time.Millisecond,
				HTTP:            &HTTP{Status: "200 OK", StatusCode: 200},
			}},
		}},
		Total:   1,
		Passed:  1,
		Failed:  0,
		Skipped: 0,
	}

	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Source Target: {{services.api.base}}/reports",
		"Effective Target: https://httpbin.org/anything/api/reports",
		"Effective Target: https://dev.httpbin.org/anything/api/reports",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in output, got %q", want, text)
		}
	}
}
