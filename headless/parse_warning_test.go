package headless

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const warnSource = `### one
# @name One
# @capture unsupported token $.token
# @sse max-event=5
GET http://127.0.0.1:1/unreachable
`

// Warnings have to reach the public report and both of its writers. They used to
// be dropped when the internal report was adapted into this package.
func TestRunExposesParseWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warn.http")
	if err := os.WriteFile(path, []byte(warnSource), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(context.Background(), Options{
		Source:    Source{Path: path},
		Selection: Selection{Request: "One"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want 2", rep.Warnings)
	}
	for _, want := range []string{`@capture scope "unsupported"`, `unknown @sse option "max-event"`} {
		if !hasWarning(rep.Warnings, want) {
			t.Fatalf("Warnings %v do not mention %q", rep.Warnings, want)
		}
	}

	var js bytes.Buffer
	if err := rep.WriteJSON(&js); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	for _, warn := range rep.Warnings {
		if !strings.Contains(js.String(), jsonEscaped(warn)) {
			t.Fatalf("WriteJSON omits %q:\n%s", warn, js.String())
		}
	}

	var text bytes.Buffer
	if err := rep.WriteText(&text); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	for _, warn := range rep.Warnings {
		if !strings.Contains(text.String(), warn) {
			t.Fatalf("WriteText omits %q:\n%s", warn, text.String())
		}
	}

	raw, err := rep.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(raw), `"warnings"`) {
		t.Fatalf("MarshalJSON omits warnings:\n%s", raw)
	}

	// A warning is advisory. The run's own outcome decides failure.
	if rep.HasFailures() != (rep.Failed > 0) {
		t.Fatalf("HasFailures=%v with Failed=%d", rep.HasFailures(), rep.Failed)
	}
}

func TestRunLeavesWarningsEmptyForACleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.http")
	src := "### one\n# @name One\nGET http://127.0.0.1:1/unreachable\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(context.Background(), Options{
		Source:    Source{Path: path},
		Selection: Selection{Request: "One"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Warnings) != 0 {
		t.Fatalf("Warnings = %v, want none", rep.Warnings)
	}
}

func hasWarning(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func jsonEscaped(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

// A Plan is documented as reusable across RunPlan calls, so a caller that keeps
// and edits one report must not affect the next run.
func TestPlanReuseGivesEachReportItsOwnWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warn.http")
	if err := os.WriteFile(path, []byte(warnSource), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{
		Source:    Source{Path: path},
		Selection: Selection{Request: "One"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	first, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(first.Warnings) == 0 {
		t.Fatal("expected warnings on the first run")
	}
	want := first.Warnings[0]
	first.Warnings[0] = "caller scribbled here"

	second, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(second.Warnings) == 0 {
		t.Fatal("expected warnings on the second run")
	}
	if second.Warnings[0] != want {
		t.Fatalf("second run warning = %q, want %q", second.Warnings[0], want)
	}
}
