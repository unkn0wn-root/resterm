package runfmt

import (
	"strings"
	"testing"
	"time"
)

func TestWriteJUnitIncludesTargetDetailsInSystemOut(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Kind:            "request",
			Name:            "reports",
			Method:          "GET",
			Target:          "{{services.api.base}}/reports",
			EffectiveTarget: "https://httpbin.org/anything/api/reports",
			Status:          StatusPass,
			Duration:        463 * time.Millisecond,
			HTTP:            &HTTP{Status: "200 OK", StatusCode: 200},
		}},
	}

	var out strings.Builder
	if err := WriteJUnit(&out, rep); err != nil {
		t.Fatalf("WriteJUnit(...): %v", err)
	}

	xml := out.String()
	for _, want := range []string{
		`<testcase name="reports" classname="GET reports" time="0.463">`,
		`Source Target: {{services.api.base}}/reports`,
		`Effective Target: https://httpbin.org/anything/api/reports`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("expected %q in output, got %q", want, xml)
		}
	}
}
