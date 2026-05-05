package runfmt

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	runfail "github.com/unkn0wn-root/resterm/internal/runx/fail"
)

func TestWriteJSONIncludesFailureChainWithoutErrorDetailDuplicate(t *testing.T) {
	err := testNetworkFailureError()
	rep := errorDetailReport(err)

	var out strings.Builder
	if err := WriteJSON(&out, rep); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}

	var got struct {
		SchemaVersion string `json:"schemaVersion"`
		Results       []struct {
			Error   string `json:"error"`
			Failure struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Chain   []struct {
					Message  string `json:"message"`
					Children []struct {
						Message  string `json:"message"`
						Children []struct {
							Message string `json:"message"`
						} `json:"children"`
					} `json:"children"`
				} `json:"chain"`
			} `json:"failure"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if got.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", got.SchemaVersion, ReportSchemaVersion)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}
	if strings.Contains(out.String(), "errorDetail") ||
		strings.Contains(out.String(), "scriptErrorDetail") ||
		strings.Contains(out.String(), "errorDiagnostic") ||
		strings.Contains(out.String(), "scriptErrorDiagnostic") ||
		strings.Contains(out.String(), "rendered") {
		t.Fatalf("json should not contain duplicate error detail objects, got %s", out.String())
	}
	res := got.Results[0]
	if res.Error == "" ||
		res.Failure.Code != string(diag.ClassNetwork) ||
		res.Failure.Message != "" {
		t.Fatalf("unexpected failure json: %+v", res)
	}
	if len(res.Failure.Chain) != 1 ||
		res.Failure.Chain[0].Message != "perform request" ||
		len(res.Failure.Chain[0].Children) != 1 ||
		res.Failure.Chain[0].Children[0].Message != `Get "https://api.local"` ||
		len(res.Failure.Chain[0].Children[0].Children) != 1 ||
		res.Failure.Chain[0].Children[0].Children[0].Message != "lookup api.local: no such host" {
		t.Fatalf("unexpected failure chain: %+v", res.Failure.Chain)
	}
}

func TestWriteTextIncludesErrorDetailBlock(t *testing.T) {
	err := testNetworkFailureError()
	rep := errorDetailReport(err)

	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Error:",
		"error[network]: request failed",
		"perform request",
		"╰─> Get \"https://api.local\"",
		"    ╰─> lookup api.local: no such host",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in output, got %q", want, text)
		}
	}
}

func TestWriteJUnitUsesRenderedErrorDetailBody(t *testing.T) {
	err := testNetworkFailureError()
	rep := errorDetailReport(err)

	var out strings.Builder
	if err := WriteJUnit(&out, rep); err != nil {
		t.Fatalf("WriteJUnit(...): %v", err)
	}

	xml := out.String()
	for _, want := range []string{
		`<failure message="perform request: Get &#34;https://api.local&#34;: lookup api.local: no such host">`,
		"error[network]: request failed",
		"perform request",
		"╰─&gt; Get &#34;https://api.local&#34;",
		"    ╰─&gt; lookup api.local: no such host",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("expected %q in output, got %q", want, xml)
		}
	}
}

func errorDetailReport(err error) *Report {
	failure := runfail.FromErrorSource(err, "error")
	detail := ErrorDetailFromError(err)
	return &Report{
		FilePath: "api.http",
		Results: []Result{{
			Kind:        "request",
			Name:        "lookup",
			Method:      "GET",
			Target:      "https://api.local",
			Status:      StatusFail,
			Error:       err.Error(),
			ErrorDetail: detail,
			Failure:     AttachErrorDetail(FromFailure(failure), detail),
		}},
		Total:  1,
		Failed: 1,
	}
}

func testNetworkFailureError() error {
	return diag.Wrap(
		&url.Error{
			Op:  "Get",
			URL: "https://api.local",
			Err: &net.DNSError{Err: "no such host", Name: "api.local"},
		},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)
}
