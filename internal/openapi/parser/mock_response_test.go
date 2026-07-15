package parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/openapi"
)

func TestParseAllResponseExamplesAndHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.yaml")
	spec := `openapi: 3.1.0
info:
  title: Demo
  version: "1"
paths:
  /payments:
    post:
      operationId: createPayment
      responses:
        "202":
          description: Accepted
          headers:
            X-Rate-Limit:
              schema:
                type: integer
                default: 10
          content:
            application/json:
              schema:
                type: object
              examples:
                review:
                  summary: Manual review
                  value: {status: review}
                accepted:
                  value: {status: pending}
                empty:
                  value: null
`
	if err := os.WriteFile(path, []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader()
	parsed, err := loader.Parse(context.Background(), path, openapi.ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	response := parsed.Operations[0].Responses[0]
	if len(response.Headers) != 1 || fmt.Sprint(response.Headers[0].Example.Value) != "10" {
		t.Fatalf("headers = %+v", response.Headers)
	}
	examples := response.MediaTypes[0].Examples
	if len(examples) != 3 || examples[0].Name != "review" || examples[1].Name != "accepted" ||
		examples[2].Name != "empty" || !examples[2].HasValue || examples[2].Value != nil {
		t.Fatalf("examples = %+v", examples)
	}
}
