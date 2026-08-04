package runfmt

import (
	"encoding/json"
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
