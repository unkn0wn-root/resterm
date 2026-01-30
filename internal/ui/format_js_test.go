package ui

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRenderJSONAsJSFormatsEmbeddedObjectStrings(t *testing.T) {
	inner := "{\n  \"token\": \"workflow-token\",\n}"
	bodyStruct := struct {
		Data string `json:"data"`
		Ok   bool   `json:"ok"`
	}{
		Data: inner,
		Ok:   true,
	}

	body, err := json.Marshal(bodyStruct)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}

	got, ok := renderJSONAsJSCtx(context.Background(), body)
	if !ok {
		t.Fatalf("renderJSONAsJSCtx returned !ok")
	}

	want := `{
  data: {
    token: "workflow-token"
  },
  ok: true
}`

	if got != want {
		t.Fatalf("unexpected pretty output\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
