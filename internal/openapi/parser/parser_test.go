package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

func TestConvertMediaTypeExplicitExamplePreferred(t *testing.T) {
	t.Parallel()

	mt := &openapi3.MediaType{
		Example: "explicit",
		Schema: &openapi3.SchemaRef{
			Ref: "#/components/schemas/Demo",
			Value: &openapi3.Schema{
				Default: "schema-default",
			},
		},
	}

	media, ok := convertMediaType("application/json", mt)
	if !ok {
		t.Fatalf("expected media type conversion")
	}
	if media.ContentType != "application/json" {
		t.Fatalf("unexpected content type: %s", media.ContentType)
	}
	if !media.Example.HasValue || media.Example.Source != model.ExampleFromExplicit || media.Example.Value != "explicit" {
		t.Fatalf("unexpected example: %#v", media.Example)
	}
	if media.Schema == nil || media.Schema.Identifier != "#/components/schemas/Demo" {
		t.Fatalf("unexpected schema ref: %#v", media.Schema)
	}
}

func TestConvertMediaTypeSchemaFallbackExample(t *testing.T) {
	t.Parallel()

	mt := &openapi3.MediaType{
		Schema: &openapi3.SchemaRef{
			Value: &openapi3.Schema{
				Default: "schema-default",
			},
		},
	}

	media, ok := convertMediaType("application/json", mt)
	if !ok {
		t.Fatalf("expected media type conversion")
	}
	if !media.Example.HasValue || media.Example.Source != model.ExampleFromDefault || media.Example.Value != "schema-default" {
		t.Fatalf("unexpected fallback example: %#v", media.Example)
	}
}

func TestLoaderParse(t *testing.T) {
	t.Parallel()

	loader := NewLoader()
	ctx := context.Background()
	path := filepath.Join("..", "testdata", "deviceinventory.yaml")

	spec, err := loader.Parse(ctx, path, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if spec.Title != "Device Inventory API" {
		t.Fatalf("unexpected title: %q", spec.Title)
	}
	if len(spec.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(spec.Servers))
	}
	if spec.Servers[1].URL != "https://staging.example.com/v2" {
		t.Fatalf("server variable resolution failed: %s", spec.Servers[1].URL)
	}

	if len(spec.SecuritySchemes) != 3 {
		t.Fatalf("expected 3 security schemes, got %d", len(spec.SecuritySchemes))
	}
	if scheme, ok := spec.SecuritySchemes["bearerAuth"]; !ok || scheme.Type != model.SecurityHTTP ||
		scheme.Subtype != "bearer" {
		t.Fatalf("bearer scheme not parsed correctly: %#v", scheme)
	}
	if scheme, ok := spec.SecuritySchemes["oauthDemo"]; !ok || scheme.Type != model.SecurityOAuth2 {
		t.Fatalf("oauth scheme missing or wrong type: %#v", scheme)
	} else if len(scheme.OAuthFlows) == 0 {
		t.Fatalf("expected oauth flows")
	} else if scheme.OAuthFlows[0].TokenURL != "https://auth.example.com/oauth/token" {
		t.Fatalf("unexpected oauth token url: %#v", scheme.OAuthFlows[0])
	}

	if len(spec.Operations) != 3 {
		t.Fatalf("expected 3 operations, got %d", len(spec.Operations))
	}

	var listDevices model.Operation
	for _, op := range spec.Operations {
		if op.ID == "listDevices" {
			listDevices = op
			break
		}
	}
	if listDevices.ID == "" {
		t.Fatalf("listDevices operation not found")
	}
	if listDevices.Method != model.MethodGet || listDevices.Path != "/devices" {
		t.Fatalf("unexpected method/path: %s %s", listDevices.Method, listDevices.Path)
	}
	if len(listDevices.Parameters) < 3 {
		t.Fatalf("expected merged parameters, got %#v", listDevices.Parameters)
	}

	paramMap := make(map[string]model.Parameter)
	for _, p := range listDevices.Parameters {
		paramMap[string(p.Location)+":"+p.Name] = p
	}
	if _, ok := paramMap["header:traceId"]; !ok {
		t.Fatalf("missing path-level header parameter")
	}
	if limit, ok := paramMap["query:limit"]; !ok || !limit.Example.HasValue {
		t.Fatalf("limit parameter example missing: %#v", limit)
	}
	if header, ok := paramMap["header:x-region"]; !ok || !header.Example.HasValue {
		t.Fatalf("header example not derived from enum")
	}

	if listDevices.Security == nil || len(listDevices.Security) != 1 ||
		listDevices.Security[0].SchemeName != "bearerAuth" {
		t.Fatalf("listDevices security not captured: %#v", listDevices.Security)
	}

	var registerDevice model.Operation
	for _, op := range spec.Operations {
		if op.ID == "registerDevice" {
			registerDevice = op
			break
		}
	}
	if registerDevice.ID == "" {
		t.Fatalf("registerDevice operation missing")
	}
	if registerDevice.RequestBody == nil || len(registerDevice.RequestBody.MediaTypes) == 0 {
		t.Fatalf("request body not parsed for registerDevice")
	}
	mt := registerDevice.RequestBody.MediaTypes[0]
	if !mt.Example.HasValue {
		t.Fatalf("request body example missing")
	}
	if len(registerDevice.Security) == 0 || registerDevice.Security[0].SchemeName != "oauthDemo" {
		t.Fatalf("registerDevice oauth security missing: %#v", registerDevice.Security)
	}

	var getDevice model.Operation
	for _, op := range spec.Operations {
		if op.ID == "getDevice" {
			getDevice = op
			break
		}
	}
	if getDevice.ID == "" {
		t.Fatalf("getDevice operation missing")
	}
	if getDevice.Security == nil || getDevice.Security[0].SchemeName != "apiKeyAuth" {
		t.Fatalf("operation-specific security not parsed: %#v", getDevice.Security)
	}
}

func TestLoaderParseHeaderRefParamCompat(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Header Compat
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/parameters/credential'
components:
  parameters:
    credential:
      name: credential
      in: header
      required: true
      schema:
        type: string
`
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	got, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got == nil {
		t.Fatalf("Parse() returned nil spec")
	}
	if len(got.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(got.Operations))
	}

	ws := loader.Warnings()
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(ws), ws)
	}
	if !strings.Contains(ws[0], "#/components/parameters/credential") {
		t.Fatalf("warning missing rewritten ref detail: %q", ws[0])
	}
}

func TestLoaderParseHeaderRefParamCompatRejectsNonHeaderParam(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Header Compat Reject
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/parameters/credential'
components:
  parameters:
    credential:
      name: credential
      in: query
      schema:
        type: string
`
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err == nil {
		t.Fatalf("expected Parse() error for non-header parameter ref")
	}
	if !strings.Contains(err.Error(), "expecting ref to header object") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "compat fallback: no header-ref rewrite candidates found") {
		t.Fatalf("expected compat fallback detail in error, got: %v", err)
	}
	if len(loader.Warnings()) != 0 {
		t.Fatalf("expected no warnings on failed rewrite, got %v", loader.Warnings())
	}
}

func TestLoaderParseHeaderRefParamCompatViaComponentsHeaders(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Header Compat Alias
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/headers/credential'
components:
  headers:
    credential:
      $ref: '#/components/parameters/credential'
  parameters:
    credential:
      name: credential
      in: header
      required: true
      schema:
        type: string
`
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	got, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got == nil || len(got.Operations) != 1 {
		t.Fatalf("expected parsed operation, got %#v", got)
	}

	ws := loader.Warnings()
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(ws), ws)
	}
	if !strings.Contains(ws[0], "#/components/headers/credential") {
		t.Fatalf("warning missing component-header location: %q", ws[0])
	}
}

func TestFixHdrRefsKeepsRefExtensions(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Header Compat Extensions
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/parameters/credential'
              x-note: keep-this
components:
  parameters:
    credential:
      name: credential
      in: header
      schema:
        type: string
`

	out, _, n, err := fixHdrRefs([]byte(spec))
	if err != nil {
		t.Fatalf("fixHdrRefs() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("expected one rewritten ref, got %d", n)
	}

	var root map[string]any
	if err := yaml.Unmarshal(out, &root); err != nil {
		t.Fatalf("unmarshal rewritten spec: %v", err)
	}

	paths := root["paths"].(map[string]any)
	demo := paths["/demo"].(map[string]any)
	get := demo["get"].(map[string]any)
	res := get["responses"].(map[string]any)
	r200 := res["200"].(map[string]any)
	hs := r200["headers"].(map[string]any)
	cred := hs["credential"].(map[string]any)

	if got := cred["x-note"]; got != "keep-this" {
		t.Fatalf("expected x-note to be preserved, got %#v", got)
	}
	if _, has := cred["$ref"]; has {
		t.Fatalf("expected $ref to be removed from rewritten header")
	}
}

func TestLoaderParseHeaderRefParamCompatWithParameterAlias(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Header Compat Param Alias
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/parameters/credentialAlias'
components:
  parameters:
    credentialCore:
      name: credential
      in: header
      required: true
      schema:
        type: string
    credentialAlias:
      $ref: '#/components/parameters/credentialCore'
`
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	got, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got == nil || len(got.Operations) != 1 {
		t.Fatalf("expected parsed operation, got %#v", got)
	}

	ws := loader.Warnings()
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(ws), ws)
	}
	if !strings.Contains(ws[0], "#/components/parameters/credentialAlias") {
		t.Fatalf("warning missing alias ref detail: %q", ws[0])
	}
}

func TestLoaderParseHeaderRefParamCompatJSON(t *testing.T) {
	t.Parallel()

	spec := `{
  "openapi": "3.0.3",
  "info": {"title": "Header Compat JSON", "version": "1.0.0"},
  "paths": {
    "/demo": {
      "get": {
        "responses": {
          "200": {
            "description": "ok",
            "headers": {
              "credential": {"$ref": "#/components/parameters/credential"}
            }
          }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "credential": {
        "name": "credential",
        "in": "header",
        "required": true,
        "schema": {"type": "string"}
      }
    }
  }
}`

	p := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	got, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got == nil || len(got.Operations) != 1 {
		t.Fatalf("expected parsed operation, got %#v", got)
	}

	ws := loader.Warnings()
	if len(ws) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(ws), ws)
	}
}

func TestFixHdrRefsKeepsJSONEncoding(t *testing.T) {
	t.Parallel()

	spec := `{
  "openapi": "3.0.3",
  "info": {"title": "Header Compat JSON", "version": "1.0.0"},
  "paths": {
    "/demo": {
      "get": {
        "responses": {
          "200": {
            "description": "ok",
            "headers": {
              "credential": {"$ref": "#/components/parameters/credential"}
            }
          }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "credential": {
        "name": "credential",
        "in": "header",
        "required": true,
        "schema": {"type": "string"}
      }
    }
  }
}`

	out, _, n, err := fixHdrRefs([]byte(spec))
	if err != nil {
		t.Fatalf("fixHdrRefs() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("expected one rewritten ref, got %d", n)
	}
	trimmed := strings.TrimSpace(string(out))
	if !strings.HasPrefix(trimmed, "{") {
		t.Fatalf("expected JSON output, got: %q", trimmed)
	}
}

func TestLoaderParseHeaderRefParamCompatRewriteErrorDetails(t *testing.T) {
	t.Parallel()

	spec := `openapi: 3.0.3
info:
  title: Broken Spec
  version: 1.0.0
paths:
  /demo:
    get:
      responses:
        '200':
          description: ok
          headers:
            credential:
              $ref: '#/components/parameters/credential'
components:
  parameters:
    credential: [
`
	p := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Parse(context.Background(), p, openapi.ParseOptions{})
	if err == nil {
		t.Fatalf("expected Parse() error")
	}
	if !strings.Contains(err.Error(), "compat fallback: rewrite spec") {
		t.Fatalf("expected compat rewrite detail in error, got: %v", err)
	}
}
