package generator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/openapi/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestBuilderGenerate(t *testing.T) {
	t.Parallel()

	loader := parser.NewLoader()
	specPath := filepath.Join("..", "testdata", "deviceinventory.yaml")
	spec, err := loader.Parse(context.Background(), specPath, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	builder := NewBuilder()
	doc, err := builder.Generate(context.Background(), spec, openapi.GeneratorOptions{})
	if err != nil {
		t.Fatalf("generate document: %v", err)
	}

	if len(doc.Requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(doc.Requests))
	}

	if len(doc.Variables) == 0 || doc.Variables[0].Name != openapi.DefaultBaseURLVariable ||
		doc.Variables[0].Value != "https://api.example.com/v1" {
		t.Fatalf("unexpected base variable: %#v", doc.Variables)
	}
	if len(doc.Globals) == 0 {
		t.Fatalf("expected global variables for auth placeholders")
	}
	globals := make(map[string]restfile.Variable)
	for _, v := range doc.Globals {
		globals[v.Name] = v
	}
	if _, ok := globals["auth.token"]; !ok {
		t.Fatalf("missing auth.token global")
	}
	if v, ok := globals["oauth.clientSecret"]; !ok || !v.Secret {
		t.Fatalf("missing oauth.clientSecret global or secret flag not set")
	}

	listReq := findRequestByName(t, doc, "listDevices")
	if listReq.Method != "GET" {
		t.Fatalf("unexpected method: %s", listReq.Method)
	}
	if want := "{{baseUrl}}/devices?limit={{query_limit}}"; listReq.URL != want {
		t.Fatalf("unexpected URL: %s", listReq.URL)
	}
	if listReq.Metadata.Auth == nil || listReq.Metadata.Auth.Type != "bearer" {
		t.Fatalf("bearer auth not configured")
	}
	if listReq.Headers.Get("Accept") != "application/json" {
		t.Fatalf("expected Accept header, got %s", listReq.Headers.Get("Accept"))
	}
	if listReq.Headers.Get("X-Region") != "{{header_x_region}}" {
		t.Fatalf("header parameter not propagated")
	}
	if _, ok := findVariable(listReq.Variables, "query_limit"); !ok {
		t.Fatalf("query parameter variable missing")
	}

	getReq := findRequestByName(t, doc, "getDevice")
	if !strings.Contains(getReq.URL, "{{path_id}}") {
		t.Fatalf("path variable not substituted: %s", getReq.URL)
	}
	if getReq.Metadata.Auth == nil || getReq.Metadata.Auth.Type != "apikey" {
		t.Fatalf("api key auth not configured")
	}

	createReq := findRequestByName(t, doc, "registerDevice")
	if createReq.Method != "POST" {
		t.Fatalf("unexpected method for registerDevice: %s", createReq.Method)
	}
	if createReq.Headers.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type header")
	}
	body := strings.TrimSpace(createReq.Body.Text)
	expectedBody := "{\n  \"model\": \"FlowSensor\",\n  \"serialNumber\": \"SN-2005\",\n  \"status\": \"active\"\n}"
	if body != expectedBody {
		t.Fatalf("unexpected body:\n%s", body)
	}
	if createReq.Metadata.Auth == nil || createReq.Metadata.Auth.Type != "oauth2" {
		t.Fatalf("expected oauth2 auth metadata: %#v", createReq.Metadata.Auth)
	}
	params := createReq.Metadata.Auth.Params
	if params["token_url"] != "https://auth.example.com/oauth/token" {
		t.Fatalf("unexpected token_url: %s", params["token_url"])
	}
	if grant := params[openapi.OAuthParamGrant]; grant != openapi.OAuthGrantClientCredentials {
		t.Fatalf("unexpected grant: %s", grant)
	}
	if scope := params[openapi.OAuthParamScope]; scope != "devices.write" {
		t.Fatalf("unexpected scope: %s", scope)
	}
}

func TestBuilderGenerateQueryParameterStyles(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	explode := true

	arraySchema := &openapi3.Schema{
		Type:  &openapi3.Types{"array"},
		Items: &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
	}
	objectSchema := &openapi3.Schema{
		Type: &openapi3.Types{"object"},
		Properties: map[string]*openapi3.SchemaRef{
			"name":  {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
			"owner": {Value: &openapi3.Schema{Type: &openapi3.Types{"string"}}},
		},
	}

	spec := &model.Spec{
		Operations: []model.Operation{
			{
				Method: model.MethodGet,
				Path:   "/items",
				Parameters: []model.Parameter{
					{
						Name:     "tags",
						Location: model.InQuery,
						Style:    "form",
						Explode:  &explode,
						Example:  model.Example{Value: []any{"red", "blue"}, HasValue: true},
						Schema: &model.SchemaRef{
							Payload: &openapi3.SchemaRef{Value: arraySchema},
						},
					},
					{
						Name:     "filters",
						Location: model.InQuery,
						Style:    "deepObject",
						Explode:  &explode,
						Example: model.Example{
							Value:    map[string]any{"name": "gizmo", "owner": "alice"},
							HasValue: true,
						},
						Schema: &model.SchemaRef{
							Payload: &openapi3.SchemaRef{Value: objectSchema},
						},
					},
				},
			},
		},
	}

	doc, err := builder.Generate(context.Background(), spec, openapi.GeneratorOptions{})
	if err != nil {
		t.Fatalf("generate spec: %v", err)
	}

	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	expectedURL := "/items?{{query_filters}}&{{query_tags}}"
	if req.URL != expectedURL {
		t.Fatalf("unexpected URL: %s", req.URL)
	}

	vars := make(map[string]restfile.Variable)
	for _, v := range req.Variables {
		vars[v.Name] = v
	}

	if value, ok := vars["query_tags"]; !ok {
		t.Fatalf("missing query_tags variable")
	} else if value.Value != "tags=red&tags=blue" {
		t.Fatalf("unexpected query_tags value: %s", value.Value)
	}

	if value, ok := vars["query_filters"]; !ok {
		t.Fatalf("missing query_filters variable")
	} else if value.Value != "filters[name]=gizmo&filters[owner]=alice" {
		t.Fatalf("unexpected query_filters value: %s", value.Value)
	}
}

func TestBuilderGenerateOAuthAuthorizationCode(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	scheme := model.SecurityScheme{
		Type: model.SecurityOAuth2,
		OAuthFlows: []model.OAuthFlow{
			{
				Type:             model.OAuthFlowAuthorizationCode,
				AuthorizationURL: "https://example.com/auth",
				TokenURL:         "https://example.com/token",
			},
		},
	}

	spec := &model.Spec{
		SecuritySchemes: map[string]model.SecurityScheme{
			"authCode": scheme,
		},
		Operations: []model.Operation{
			{
				Method:   model.MethodGet,
				Path:     "/secure",
				Security: []model.SecurityRequirement{{SchemeName: "authCode"}},
			},
		},
	}

	doc, err := builder.Generate(context.Background(), spec, openapi.GeneratorOptions{})
	if err != nil {
		t.Fatalf("generate spec: %v", err)
	}

	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}
	req := doc.Requests[0]
	if req.Metadata.Auth == nil {
		t.Fatalf("expected auth metadata")
	}
	if req.Metadata.Auth.Type != "oauth2" {
		t.Fatalf("expected oauth2, got %s", req.Metadata.Auth.Type)
	}
	params := req.Metadata.Auth.Params
	if params[openapi.OAuthParamGrant] != openapi.OAuthGrantAuthorizationCode {
		t.Fatalf("unexpected grant %s", params[openapi.OAuthParamGrant])
	}
	if params[openapi.OAuthParamTokenURL] != "https://example.com/token" {
		t.Fatalf("unexpected token_url %s", params[openapi.OAuthParamTokenURL])
	}
	if params[openapi.OAuthParamAuthURL] != "https://example.com/auth" {
		t.Fatalf("unexpected auth_url %s", params[openapi.OAuthParamAuthURL])
	}
	if params[openapi.OAuthParamCodeMethod] != "s256" {
		t.Fatalf(
			"expected code_challenge_method s256, got %s",
			params[openapi.OAuthParamCodeMethod],
		)
	}

	wantGlobals := map[string]bool{
		"oauth.clientId":     false,
		"oauth.clientSecret": true,
	}
	for key, secret := range wantGlobals {
		found := false
		for _, g := range doc.Globals {
			if g.Name == key {
				found = true
				if g.Secret != secret {
					t.Fatalf("unexpected secret flag for %s", key)
				}
			}
		}
		if !found {
			t.Fatalf("expected global %s", key)
		}
	}

	if warnings := builder.Warnings(); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
}

func findRequestByName(t *testing.T, doc *restfile.Document, name string) *restfile.Request {
	t.Helper()
	for _, req := range doc.Requests {
		if req.Metadata.Name == name {
			return req
		}
	}
	t.Fatalf("request %s not found", name)
	return nil
}

func findVariable(vars []restfile.Variable, name string) (restfile.Variable, bool) {
	for _, v := range vars {
		if v.Name == name {
			return v, true
		}
	}
	return restfile.Variable{}, false
}
