package ui

import (
	"context"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestRunAsserts(t *testing.T) {
	model := New(Config{})
	doc := &restfile.Document{Path: "assert.http"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Asserts: []restfile.AssertSpec{
				{Expression: "status == 200", Line: 1},
				{Expression: `contains(header("Content-Type"), "json")`, Line: 2},
				{Expression: "status == 201", Line: 3, Message: "should fail"},
			},
		},
	}
	resp := &rts.Resp{
		Status: "200 OK",
		Code:   200,
		H:      map[string][]string{"Content-Type": {"application/json"}},
		Body:   []byte(`{"ok":true}`),
	}

	results, err := model.runAsserts(
		context.Background(),
		doc,
		req,
		"",
		"",
		map[string]string{},
		nil,
		resp,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("run asserts: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Passed || !results[1].Passed {
		t.Fatalf("expected first two asserts to pass, got %+v", results)
	}
	if results[2].Passed {
		t.Fatalf("expected third assert to fail, got %+v", results[2])
	}
	if results[2].Message != "should fail" {
		t.Fatalf("unexpected assert message: %q", results[2].Message)
	}
}
