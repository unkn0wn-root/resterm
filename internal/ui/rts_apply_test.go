package ui

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunRTSApplyPatch(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/path?keep=1&del=2",
		Headers: http.Header{
			"X-Old":  []string{"old"},
			"X-Keep": []string{"keep"},
		},
		Variables: []restfile.Variable{
			{Name: "old", Value: "x", Scope: restfile.ScopeRequest},
		},
		LineRange: restfile.LineRange{Start: 1, End: 4},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Expression: `{method: "post", url: "https://example.com/new?seed=1", headers: {"X-Test": "1", "X-Old": null, "X-List": ["a", "b"]}, query: {"q": "a", "keep": null}, body: {a: 1}, auth: {type: "oauth2", cache_key: "myapi"}, settings: {timeout: "3s", followredirects: false}, vars: {"token": "abc"}}`,
				Line:       1,
				Col:        1,
			}},
		},
	}
	vars := map[string]string{"existing": "1"}

	if err := model.runRTSApply(context.Background(), nil, req, "", "", vars, nil); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}

	if req.Method != "POST" {
		t.Fatalf("expected method POST, got %q", req.Method)
	}

	parsed, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "example.com" || parsed.Path != "/new" {
		t.Fatalf("unexpected url after apply: %s", req.URL)
	}
	query := parsed.Query()
	if query.Get("q") != "a" || query.Get("seed") != "1" {
		t.Fatalf("unexpected query values: %v", query)
	}
	if _, ok := query["keep"]; ok {
		t.Fatalf("expected keep query param deleted")
	}

	if got := req.Headers.Get("X-Test"); got != "1" {
		t.Fatalf("expected X-Test header 1, got %q", got)
	}
	if _, ok := req.Headers["X-Old"]; ok {
		t.Fatalf("expected X-Old header deleted")
	}
	if got := req.Headers.Get("X-Keep"); got != "keep" {
		t.Fatalf("expected X-Keep header preserved, got %q", got)
	}
	if got := req.Headers.Values("X-List"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected X-List header values [a b], got %#v", got)
	}

	if req.Body.Text != `{"a":1}` {
		t.Fatalf("expected body %q, got %q", `{"a":1}`, req.Body.Text)
	}
	if strings.TrimSpace(req.Body.FilePath) != "" {
		t.Fatalf("expected body file cleared, got %q", req.Body.FilePath)
	}
	if req.Metadata.Auth == nil {
		t.Fatalf("expected auth to be set")
	}
	if req.Metadata.Auth.Type != "oauth2" {
		t.Fatalf("expected oauth2 auth type, got %q", req.Metadata.Auth.Type)
	}
	if req.Metadata.Auth.Params["cache_key"] != "myapi" {
		t.Fatalf("expected cache_key myapi, got %q", req.Metadata.Auth.Params["cache_key"])
	}
	if req.Settings["timeout"] != "3s" {
		t.Fatalf("expected timeout setting 3s, got %q", req.Settings["timeout"])
	}
	if req.Settings["followredirects"] != "false" {
		t.Fatalf("expected followredirects false, got %q", req.Settings["followredirects"])
	}

	if vars["token"] != "abc" || vars["existing"] != "1" {
		t.Fatalf("unexpected vars map: %#v", vars)
	}
	if _, ok := findReqVar(req, "token"); !ok {
		t.Fatalf("expected request variable token to be set")
	}
	if _, ok := findReqVar(req, "old"); !ok {
		t.Fatalf("expected existing request variable to be preserved")
	}
}

func TestRunRTSApplyOrder(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 1, End: 4},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{
				{Expression: `{headers: {"X-Test": "1"}}`, Line: 1, Col: 1},
				{
					Expression: `{headers: {"X-Next": request.header("X-Test") + "b"}}`,
					Line:       2,
					Col:        1,
				},
			},
		},
	}

	if err := model.runRTSApply(
		context.Background(),
		nil,
		req,
		"",
		"",
		map[string]string{},
		nil,
	); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}
	if got := req.Headers.Get("X-Test"); got != "1" {
		t.Fatalf("expected X-Test header 1, got %q", got)
	}
	if got := req.Headers.Get("X-Next"); got != "1b" {
		t.Fatalf("expected X-Next header 1b, got %q", got)
	}
}

func TestRunRTSApplyClearsAuthAndDeletesSetting(t *testing.T) {
	m := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 1, End: 2},
		Settings:  map[string]string{"timeout": "5s"},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "old"},
			},
			Applies: []restfile.ApplySpec{{
				Expression: `{auth: null, settings: {timeout: null}}`,
				Line:       1,
				Col:        1,
			}},
		},
	}

	if err := m.runRTSApply(context.Background(), nil, req, "", "", nil, nil); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}
	if req.Metadata.Auth != nil {
		t.Fatalf("expected auth to be cleared")
	}
	if _, ok := req.Settings["timeout"]; ok {
		t.Fatalf("expected timeout setting to be deleted")
	}
}

func TestRunRTSApplyTemplatedURLQuery(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://{{host}}/path?keep=1#frag",
		LineRange: restfile.LineRange{Start: 1, End: 3},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Expression: `{query: {"q": "a", "keep": null}}`,
				Line:       1,
				Col:        1,
			}},
		},
	}

	if err := model.runRTSApply(context.Background(), nil, req, "", "", nil, nil); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}
	if !strings.Contains(req.URL, "{{host}}") {
		t.Fatalf("expected template to be preserved, got %q", req.URL)
	}
	if !strings.Contains(req.URL, "#frag") {
		t.Fatalf("expected fragment to be preserved, got %q", req.URL)
	}
	q := queryFromURL(req.URL)
	vals, err := url.ParseQuery(q)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if vals.Get("q") != "a" {
		t.Fatalf("expected query q=a, got %q", vals.Get("q"))
	}
	if _, ok := vals["keep"]; ok {
		t.Fatalf("expected keep query param deleted")
	}
}

func TestRunRTSApplyTemplatedQueryPreservesTemplate(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com/path?mode={{= helpers.mode(env) }}&keep=1",
		LineRange: restfile.LineRange{Start: 1, End: 3},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Expression: `{query: {"q": "x", "keep": null}}`,
				Line:       1,
				Col:        1,
			}},
		},
	}

	if err := model.runRTSApply(context.Background(), nil, req, "", "", nil, nil); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}
	if !strings.Contains(req.URL, "{{= helpers.mode(env) }}") {
		t.Fatalf("expected template to be preserved, got %q", req.URL)
	}
	if strings.Contains(req.URL, "keep=1") {
		t.Fatalf("expected keep query param deleted, got %q", req.URL)
	}
	if !strings.Contains(req.URL, "q=x") {
		t.Fatalf("expected query q=x, got %q", req.URL)
	}
}

func TestRunRTSApplyUseProfiles(t *testing.T) {
	m := New(Config{})
	doc := &restfile.Document{
		Path: "/tmp/patches.http",
		Patches: []restfile.PatchProfile{
			{
				Scope:      restfile.PatchScopeFile,
				Name:       "jsonApi",
				Expression: `{method: "post", headers: {"X-From": "file"}}`,
				Line:       1,
				Col:        1,
			},
		},
	}
	if m.patchGlobals != nil {
		m.patchGlobals.set("/tmp/other.http", []restfile.PatchProfile{
			{
				Scope:      restfile.PatchScopeGlobal,
				Name:       "jsonApi",
				Expression: `{method: "delete", headers: {"X-From": "global"}}`,
			},
			{
				Scope:      restfile.PatchScopeGlobal,
				Name:       "authProd",
				Expression: `{headers: {"Authorization": "Bearer abc"}}`,
			},
		})
	}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 5, End: 8},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Uses: []string{"jsonApi", "authProd"},
				Line: 5,
				Col:  1,
			}},
		},
	}

	if err := m.runRTSApply(context.Background(), doc, req, "", "", nil, nil); err != nil {
		t.Fatalf("runRTSApply: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("expected file patch to win and set method POST, got %q", req.Method)
	}
	if got := req.Headers.Get("X-From"); got != "file" {
		t.Fatalf("expected file header value, got %q", got)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("expected auth header from global profile, got %q", got)
	}
}

func TestRunRTSApplyUseMissingProfile(t *testing.T) {
	m := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 1, End: 2},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Uses: []string{"missing"},
				Line: 1,
				Col:  1,
			}},
		},
	}

	err := m.runRTSApply(context.Background(), nil, req, "", "", nil, nil)
	if err == nil {
		t.Fatalf("expected missing profile error")
	}
	if !strings.Contains(err.Error(), `use="missing" not found`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func findReqVar(req *restfile.Request, name string) (restfile.Variable, bool) {
	if req == nil {
		return restfile.Variable{}, false
	}
	for _, v := range req.Variables {
		if strings.EqualFold(v.Name, name) {
			return v, true
		}
	}
	return restfile.Variable{}, false
}

func queryFromURL(raw string) string {
	idx := strings.Index(raw, "?")
	if idx == -1 {
		return ""
	}
	qs := raw[idx+1:]
	if cut := strings.Index(qs, "#"); cut >= 0 {
		qs = qs[:cut]
	}
	return qs
}
