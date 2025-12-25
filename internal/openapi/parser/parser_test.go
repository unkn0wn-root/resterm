package parser

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

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
