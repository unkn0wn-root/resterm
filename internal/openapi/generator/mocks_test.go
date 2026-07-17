package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
)

func TestGenerateOpenAPIMocks(t *testing.T) {
	spec := &model.Spec{Operations: []model.Operation{{
		ID:      "createPayment",
		Method:  model.MethodPost,
		Path:    "/payments/{payment-id}",
		Summary: "Create payment",
		Responses: []model.Response{
			{
				StatusCode: "202",
				Headers: []model.Header{{
					Name:    "X-Rate-Limit",
					Example: model.Example{Value: 10, HasValue: true},
				}},
				MediaTypes: []model.MediaType{{
					ContentType: "application/json",
					Examples: []model.Example{
						{Name: "accepted", Value: map[string]any{"status": "pending"}, HasValue: true},
						{Name: "review", Value: map[string]any{"status": "review"}, HasValue: true},
					},
				}},
			},
			{
				StatusCode: "400",
				MediaTypes: []model.MediaType{{
					ContentType: "application/json",
					Schema: &model.SchemaRef{Node: &model.Schema{
						Types: []model.SchemaType{model.TypeObject},
						Properties: map[string]*model.SchemaRef{
							"error": {Node: &model.Schema{Types: []model.SchemaType{model.TypeString}}},
						},
					}},
				}},
			},
		},
	}}}

	doc, err := NewBuilder().Generate(
		context.Background(),
		spec,
		openapi.GeneratorOptions{Mode: openapi.GenerationMocks},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Requests) != 0 || len(doc.Mocks) != 3 {
		t.Fatalf("requests=%d mocks=%d", len(doc.Requests), len(doc.Mocks))
	}
	if first := doc.Mocks[0]; !first.Default || first.Name != "status-202-accepted" ||
		first.Responses[0].Headers.Get("X-Rate-Limit") != "10" {
		t.Fatalf("first generated mock = %+v", first)
	}

	rendered, err := restwriter.Render(doc, restwriter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	parsed := parser.Parse("generated.http", []byte(rendered))
	if len(parsed.Errors) > 0 {
		t.Fatalf("rendered mocks do not parse: %+v\n%s", parsed.Errors, rendered)
	}
	if _, err := mock.Compile([]*restfile.Document{parsed}); err != nil {
		t.Fatalf("rendered mocks do not compile: %v\n%s", err, rendered)
	}
}

func TestGenerateOpenAPIBoth(t *testing.T) {
	spec := &model.Spec{Operations: []model.Operation{{
		Method: model.MethodGet,
		Path:   "/health",
		Responses: []model.Response{{
			StatusCode: "200",
		}},
	}}}
	doc, err := NewBuilder().Generate(
		context.Background(),
		spec,
		openapi.GeneratorOptions{Mode: openapi.GenerationBoth},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Requests) != 1 || len(doc.Mocks) != 1 {
		t.Fatalf("requests=%d mocks=%d", len(doc.Requests), len(doc.Mocks))
	}
}

func TestGenerateOpenAPIMockPreservesLiteralTemplates(t *testing.T) {
	spec := &model.Spec{Operations: []model.Operation{{
		Method: model.MethodGet,
		Path:   "/templates",
		Responses: []model.Response{{
			StatusCode: "200",
			MediaTypes: []model.MediaType{{
				ContentType: "application/json",
				Examples: []model.Example{{
					Value:    map[string]any{"value": "{{literal}}"},
					HasValue: true,
				}},
			}},
		}},
	}}}
	doc, err := NewBuilder().Generate(
		context.Background(),
		spec,
		openapi.GeneratorOptions{Mode: openapi.GenerationMocks},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Mocks) != 1 || !doc.Mocks[0].DisableInterpolation {
		t.Fatalf("generated mocks = %+v", doc.Mocks)
	}
	rendered, err := restwriter.Render(doc, restwriter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "interpolate=false") || !strings.Contains(rendered, "{{literal}}") {
		t.Fatalf("rendered mock:\n%s", rendered)
	}
}
