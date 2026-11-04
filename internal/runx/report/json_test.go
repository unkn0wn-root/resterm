package runfmt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestWriteJSONIncludesGRPCStatusDetails(t *testing.T) {
	detail := `{"@type":"type.googleapis.com/google.rpc.RetryInfo"}`
	rep := grpcStatusReport([]string{detail})

	var out strings.Builder
	if err := WriteJSON(&out, rep); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}

	var got struct {
		Results []struct {
			GRPC struct {
				StatusMessage string   `json:"statusMessage"`
				StatusDetails []string `json:"statusDetails"`
			} `json:"grpc"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}
	grpc := got.Results[0].GRPC
	if grpc.StatusMessage != "invalid page size" {
		t.Fatalf("statusMessage = %q, want invalid page size", grpc.StatusMessage)
	}
	if len(grpc.StatusDetails) != 1 || grpc.StatusDetails[0] != detail {
		t.Fatalf("statusDetails = %v, want [%s]", grpc.StatusDetails, detail)
	}
}

func TestWriteJSONOmitsEmptyGRPCStatusDetails(t *testing.T) {
	var out strings.Builder
	if err := WriteJSON(&out, grpcStatusReport(nil)); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}
	if strings.Contains(out.String(), "statusDetails") {
		t.Fatalf("json should omit empty status details, got %s", out.String())
	}
}

func grpcStatusReport(details []string) *Report {
	return &Report{
		FilePath: "api.http",
		Results: []Result{{
			Kind:   "request",
			Name:   "invalid request",
			Method: "GRPC",
			Status: StatusFail,
			GRPC: &GRPC{
				Code:          "InvalidArgument",
				StatusCode:    3,
				StatusMessage: "invalid page size",
				StatusDetails: details,
			},
		}},
		Total:  1,
		Failed: 1,
	}
}

func TestWriteJSONIncludesResultWarnings(t *testing.T) {
	warn := "Script sends {{token}} in header X-Id as written."
	rep := &Report{Results: []Result{{Kind: "request", Status: StatusPass, Warnings: []string{warn}}}}

	var out strings.Builder
	if err := WriteJSON(&out, rep); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}
	var got struct {
		Results []struct {
			Warnings []string `json:"warnings"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || len(got.Results[0].Warnings) != 1 || got.Results[0].Warnings[0] != warn {
		t.Fatalf("results = %+v, want one warning %q", got.Results, warn)
	}
}

func TestWriteJSONOmitsResponseByDefault(t *testing.T) {
	rep := &Report{Results: []Result{{
		Kind:   "workflow",
		Status: StatusPass,
		Steps:  []Step{{Name: "a", Status: StatusPass, HTTP: &HTTP{Status: "200 OK", StatusCode: 200}}},
	}}}

	var out strings.Builder
	if err := WriteJSON(&out, rep); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}
	if strings.Contains(out.String(), `"response"`) {
		t.Fatalf("json should omit response, got %s", out.String())
	}
}

func TestWriteJSONResponseTextBody(t *testing.T) {
	got := responseJSON(t, &Response{
		Headers:  http.Header{"Content-Type": {"application/json"}},
		Body:     []byte(`{"a":1}`),
		BodySize: 7,
	})
	if got["body"] != `{"a":1}` || got["bodySize"] != 7.0 {
		t.Fatalf("response = %v, want the body as text", got)
	}
	if _, ok := got["bodyEncoding"]; ok {
		t.Fatalf("a text body has bodyEncoding: %v", got)
	}
	if _, ok := got["bodyTruncated"]; ok {
		t.Fatalf("a whole body has bodyTruncated: %v", got)
	}
}

func TestWriteJSONResponseBinaryBody(t *testing.T) {
	body := make([]byte, 256)
	for i := range body {
		body[i] = byte(i)
	}
	got := responseJSON(t, &Response{Body: body, BodySize: 256, BodyTruncated: true})
	if got["bodyEncoding"] != "base64" || got["bodyTruncated"] != true {
		t.Fatalf("response = %v, want a base64 body", got)
	}
	text, _ := got["body"].(string)
	back, err := base64.StdEncoding.DecodeString(text)
	if err != nil || !bytes.Equal(back, body) {
		t.Fatalf("body %q decodes to %v (err %v), want the original bytes", text, back, err)
	}
}

func TestWriteJSONResponseEmptyBody(t *testing.T) {
	got := responseJSON(t, &Response{})
	if size, ok := got["bodySize"]; !ok || size != 0.0 {
		t.Fatalf("response = %v, want bodySize 0", got)
	}
	for _, key := range []string{"body", "bodyEncoding", "headers", "captures"} {
		if _, ok := got[key]; ok {
			t.Fatalf("an empty response has %q: %v", key, got)
		}
	}
}

func responseJSON(t *testing.T, resp *Response) map[string]any {
	t.Helper()
	rep := &Report{Results: []Result{{Kind: "request", Status: StatusPass, Response: resp}}}
	var out strings.Builder
	if err := WriteJSON(&out, rep); err != nil {
		t.Fatalf("WriteJSON(...): %v", err)
	}
	var got struct {
		Results []struct {
			Response map[string]any `json:"response"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Response == nil {
		t.Fatalf("expected one result with a response, got %s", out.String())
	}
	return got.Results[0].Response
}
