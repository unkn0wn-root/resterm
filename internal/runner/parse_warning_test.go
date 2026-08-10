package runner

import (
	"bytes"
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

// Unknown options and other recoverable directive problems are warnings, so the
// run has to report them without turning into a failure of its own.
func TestRunPlanReportsParseWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "warn.http")
	if err := os.WriteFile(path, []byte(warnSource), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{FilePath: path, Select: Select{Request: "One"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	rep, err := RunPlan(t.Context(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(rep.Warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", rep.Warnings)
	}
	for _, want := range []string{`@capture scope "unsupported"`, `unknown @sse option "max-event"`} {
		if !containsAny(rep.Warnings, want) {
			t.Fatalf("warnings %v do not mention %q", rep.Warnings, want)
		}
	}
	for _, warn := range rep.Warnings {
		if !strings.HasPrefix(warn, path+":") {
			t.Fatalf("warning %q does not start with the file location", warn)
		}
	}

	// The request cannot connect, so it fails on its own. The point is that the
	// warnings did not add to the count.
	if rep.Failed != 1 || rep.Passed != 0 {
		t.Fatalf("passed=%d failed=%d, want 0/1", rep.Passed, rep.Failed)
	}

	var text bytes.Buffer
	if err := rep.WriteText(&text); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	for _, warn := range rep.Warnings {
		if !strings.Contains(text.String(), warn) {
			t.Fatalf("text report omits %q:\n%s", warn, text.String())
		}
	}

	var js bytes.Buffer
	if err := rep.WriteJSON(&js); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(js.String(), `"warnings"`) {
		t.Fatalf("json report omits warnings:\n%s", js.String())
	}
}

func TestRunPlanLeavesWarningsEmptyForACleanFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.http")
	src := "### one\n# @name One\nGET http://127.0.0.1:1/unreachable\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{FilePath: path, Select: Select{Request: "One"}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	rep, err := RunPlan(t.Context(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(rep.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", rep.Warnings)
	}
}

func containsAny(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
