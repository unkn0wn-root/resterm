package ui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
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

	got, ok := bodyfmt.RenderJSONAsJSContext(context.Background(), body)
	if !ok {
		t.Fatalf("RenderJSONAsJSContext returned !ok")
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
