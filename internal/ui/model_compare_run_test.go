package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc/codes"
)

func TestCompareResultSuccess(t *testing.T) {
	cases := []struct {
		name   string
		result compareResult
		want   bool
	}{
		{
			name: "http ok",
			result: compareResult{
				Response: &httpclient.Response{StatusCode: 200},
			},
			want: true,
		},
		{
			name: "http failure status",
			result: compareResult{
				Response: &httpclient.Response{StatusCode: 500},
			},
			want: false,
		},
		{
			name:   "runtime error",
			result: compareResult{Err: errors.New("boom")},
			want:   false,
		},
		{
			name:   "script error",
			result: compareResult{ScriptErr: errors.New("tests failed")},
			want:   false,
		},
		{
			name: "grpc ok",
			result: compareResult{
				GRPC: &grpcclient.Response{StatusCode: codes.OK},
			},
			want: true,
		},
		{
			name: "grpc failure",
			result: compareResult{
				GRPC: &grpcclient.Response{StatusCode: codes.Internal},
			},
			want: false,
		},
		{
			name:   "canceled",
			result: compareResult{Canceled: true},
			want:   false,
		},
	}

	for _, tc := range cases {
		if got := compareResultSuccess(&tc.result); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestCompareStateProgressSummary(t *testing.T) {
	state := &compareState{
		label: "Compare users",
		spec:  &restfile.CompareSpec{Baseline: "dev"},
		envs:  []string{"dev", "stage", "prod"},
		results: []compareResult{
			{Environment: "dev", Response: &httpclient.Response{StatusCode: 200}},
			{Environment: "stage", Err: errors.New("boom")},
		},
		index:   2,
		current: &restfile.Request{},
	}

	wantSummary := "dev*✓ stage✗ prod…"
	if got := state.progressSummary(); got != wantSummary {
		t.Fatalf("expected %q, got %q", wantSummary, got)
	}

	wantLine := "Compare users | " + wantSummary
	if got := state.statusLine(); got != wantLine {
		t.Fatalf("expected status line %q, got %q", wantLine, got)
	}

	if !state.hasFailures() {
		t.Fatalf("expected failures detected")
	}
}

func TestBundleFromHistory(t *testing.T) {
	entry := history.Entry{
		Method: restfile.HistoryMethodCompare,
		Compare: &history.CompareEntry{
			Baseline: "dev",
			Results: []history.CompareResult{
				{
					Environment: "dev",
					Status:      "200 OK",
					StatusCode:  200,
					Duration:    10 * time.Millisecond,
				},
				{
					Environment: "stage",
					Status:      "500",
					StatusCode:  500,
					Error:       "boom",
					Duration:    12 * time.Millisecond,
				},
			},
		},
	}

	bundle := bundleFromHistory(entry)
	if bundle == nil {
		t.Fatalf("expected bundle")
	}
	if bundle.Baseline != "dev" {
		t.Fatalf("expected baseline dev, got %s", bundle.Baseline)
	}
	if len(bundle.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(bundle.Rows))
	}
	if bundle.Rows[1].Summary == "" {
		t.Fatalf("expected summary text for second row")
	}
}

func TestSelectCompareHistoryResultPrefersFailure(t *testing.T) {
	entry := history.Entry{
		Compare: &history.CompareEntry{
			Results: []history.CompareResult{
				{Environment: "dev", StatusCode: 200},
				{Environment: "stage", StatusCode: 500, Error: "boom"},
			},
		},
	}
	selected := selectCompareHistoryResult(entry)
	if selected == nil || selected.Environment != "stage" {
		t.Fatalf("expected to pick failing environment, got %#v", selected)
	}
}

func TestRecordCompareHistoryPersists(t *testing.T) {
	tmp := t.TempDir()
	store := history.NewStore(filepath.Join(tmp, "history.json"), 10)
	model := New(Config{History: store})

	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com/compare",
		Metadata: restfile.RequestMetadata{Name: "CompareRequest"},
	}

	state := &compareState{
		base:  cloneRequest(req),
		spec:  &restfile.CompareSpec{Baseline: "dev"},
		envs:  []string{"dev"},
		index: 1,
		results: []compareResult{
			{
				Environment: "dev",
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(`{"ok":true}`),
					Duration:   5 * time.Millisecond,
				},
				Request: cloneRequest(req),
			},
		},
	}

	model.recordCompareHistory(state)
	entries := store.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Method != restfile.HistoryMethodCompare {
		t.Fatalf("expected method compare, got %s", entries[0].Method)
	}
	if entries[0].Compare == nil {
		t.Fatalf("expected compare payload present")
	}
}

func TestCompareCancelStopsRun(t *testing.T) {
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com/compare",
		Metadata: restfile.RequestMetadata{Name: "CompareRequest"},
	}

	state := &compareState{
		base:       cloneRequest(req),
		spec:       &restfile.CompareSpec{Baseline: "dev"},
		envs:       []string{"dev", "stage"},
		current:    cloneRequest(req),
		currentEnv: "dev",
		label:      "Compare sample",
	}

	model := New(Config{})
	model.ready = true
	model.compareRun = state
	model.sending = true

	if follow := model.handleCompareResponse(
		responseMsg{err: context.Canceled, executed: state.current},
	); follow != nil {
		collectMsgs(follow)
	}

	if model.compareRun != nil {
		t.Fatalf("expected compare run to clear after cancel")
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected warning status for cancel, got %v", model.statusMessage.level)
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "canceled") {
		t.Fatalf("expected canceled status message, got %q", model.statusMessage.text)
	}
	if len(state.results) != 1 || state.results[0].Err != nil {
		t.Fatalf(
			"expected canceled environment to be recorded without error payload, got %+v",
			state.results,
		)
	}
	if !state.results[0].Canceled {
		t.Fatalf("expected canceled result marker")
	}
}

func TestExecuteCompareIterationSetsSending(t *testing.T) {
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com/compare",
		Metadata: restfile.RequestMetadata{Name: "CompareRequest"},
	}

	state := &compareState{
		doc:   &restfile.Document{Requests: []*restfile.Request{req}},
		base:  cloneRequest(req),
		spec:  &restfile.CompareSpec{Environments: []string{"dev", "stage"}},
		envs:  []string{"dev", "stage"},
		label: "Compare test",
	}

	model := New(Config{})
	model.ready = true
	model.compareRun = state

	cmd := model.executeCompareIteration()
	if !model.sending {
		t.Fatalf("expected compare iteration to mark sending")
	}
	if cmd == nil {
		t.Fatalf("expected iteration command to be scheduled")
	}
}
