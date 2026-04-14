package headless

import (
	"bytes"
	"strings"
	"testing"
)

func TestReportWritersAvailable(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Total:    1,
		Passed:   1,
		Results: []Result{{
			Name:   "ok",
			Status: StatusPass,
		}},
	}

	var jsonBuf bytes.Buffer
	if err := rep.WriteJSON(&jsonBuf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(jsonBuf.String(), `"filePath": "api.http"`) {
		t.Fatalf("WriteJSON output missing file path: %s", jsonBuf.String())
	}

	var textBuf bytes.Buffer
	if err := rep.WriteText(&textBuf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(textBuf.String(), "Summary: total=1 passed=1 failed=0 skipped=0") {
		t.Fatalf("WriteText output missing summary: %s", textBuf.String())
	}

	var junitBuf bytes.Buffer
	if err := rep.WriteJUnit(&junitBuf); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	if !strings.Contains(junitBuf.String(), "<testsuites") {
		t.Fatalf("WriteJUnit output missing testsuites root: %s", junitBuf.String())
	}
}
