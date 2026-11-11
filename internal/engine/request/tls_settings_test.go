package request

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"

	"github.com/unkn0wn-root/resterm/internal/directive"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	errVerify       = "tls: failed to verify certificate"
	errMissing      = "read root ca missing.pem"
	errSameInsecure = "http-insecure and insecure are the same setting. Use one of them."
	errSameRoots    = "http-root-ca and http-root-cas are the same setting. Use one of them."
	goodCA          = "good.pem"
	missingCA       = "missing.pem"
)

var (
	insecurePairs = [][2]string{
		{"http-insecure", "insecure"},
		{"insecure", "http-insecure"},
		{"insecure", "insecure"},
		{"http-insecure", "http-insecure"},
	}
	rootPairs = [][2]string{
		{"http-root-cas", "http-root-ca"},
		{"http-root-ca", "http-root-cas"},
		{"http-root-ca", "http-root-ca"},
		{"http-root-cas", "http-root-cas"},
	}
)

// The layers settings come from, lowest first. apply is an inline @apply and
// use a later @apply use= of a file patch.
type scope int

const (
	scopeShared scope = iota
	scopeEnv
	scopeFile
	scopeRequest
	scopeApply
	scopeUse
	numScopes
)

func (s scope) String() string {
	return [...]string{"shared", "env", "file", "request", "apply", "use"}[s]
}

// tlsSend is one request with the settings each scope writes. groups, when
// set, select grouped profiles in place of the env scope. An empty want means
// the request succeeds.
type tlsSend struct {
	name   string
	set    [numScopes]map[string]string
	groups []vars.Group
	want   string
}

func (s tlsSend) at(sc scope, key, val string) tlsSend {
	m := maps.Clone(s.set[sc])
	if m == nil {
		m = map[string]string{}
	}
	m[key] = val
	s.set[sc] = m
	if s.name != "" {
		s.name += ", "
	}
	s.name += fmt.Sprintf("%s %s=%s", sc, key, val)
	return s
}

func (s tlsSend) exec(
	t *testing.T,
	doc *restfile.Document,
	req *restfile.Request,
	opt ExecOptions,
) engcfg.RequestResult {
	t.Helper()

	envs := vars.EnvironmentSet{vars.SharedEnvKey: {}, "dev": {}}
	for sc, name := range map[scope]string{scopeShared: vars.SharedEnvKey, scopeEnv: "dev"} {
		for k, v := range s.set[sc] {
			envs[name]["settings."+k] = v
		}
	}
	var cat vars.Catalog
	var err error
	if s.groups != nil {
		cat, err = vars.NewGroupedCatalog(envs[vars.SharedEnvKey], s.groups)
	} else {
		cat, err = vars.NewCatalog(envs)
	}
	if err != nil {
		t.Fatal(err)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatal(err)
	}

	doc.Settings = s.set[scopeFile]
	req.Settings = s.set[scopeRequest]
	if set := s.set[scopeApply]; set != nil {
		req.Metadata.Applies = append(req.Metadata.Applies, restfile.ApplySpec{Expression: settingsPatch(set)})
	}
	if set := s.set[scopeUse]; set != nil {
		p := restfile.PatchProfile{Scope: directive.ScopeFile, Name: "use", Expression: settingsPatch(set)}
		doc.Patches = append(doc.Patches, p)
		req.Metadata.Applies = append(req.Metadata.Applies, restfile.ApplySpec{Uses: []string{p.Name}})
	}
	res, err := New(engcfg.Config{}, nil).ExecuteWith(doc, req, env, opt)
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	return res
}

func (s tlsSend) run(t *testing.T, doc *restfile.Document, req *restfile.Request) {
	t.Helper()

	res := s.exec(t, doc, req, ExecOptions{})
	switch {
	case s.want == "" && res.Err != nil:
		t.Fatalf("request failed: %v", res.Err)
	case s.want != "" && (res.Err == nil || !strings.Contains(res.Err.Error(), s.want)):
		t.Fatalf("error = %v, want it to contain %q", res.Err, s.want)
	}
}

// settingsPatch writes set as an @apply patch.
func settingsPatch(set map[string]string) string {
	b, _ := json.Marshal(set)
	return "{settings: " + string(b) + "}"
}

func writeCA(t *testing.T, dir string, srv *httptest.Server) {
	t.Helper()

	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(filepath.Join(dir, goodCA), ca, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Root CA paths are relative to the file, so the error for the missing one
// names the value that won.
func TestExecuteTLSSettingNamesFollowScope(t *testing.T) {
	const (
		on    = "RESTERM_TLS_TEST_ON"
		unset = "RESTERM_TLS_TEST_UNSET"
	)
	t.Setenv(on, "true")
	t.Setenv(unset, "")
	if err := os.Unsetenv(unset); err != nil {
		t.Fatal(err)
	}

	var seen atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	// Counts connections and requests, so a settings error can show it sent nothing.
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew || st == http.StateActive {
			seen.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	writeCA(t, dir, srv)

	var sends []tlsSend
	for lo := range numScopes {
		for hi := lo + 1; hi < numScopes; hi++ {
			for _, k := range insecurePairs {
				sends = append(sends,
					tlsSend{}.at(lo, k[0], "false").at(hi, k[1], "true"),
					tlsSend{want: errVerify}.at(lo, k[0], "true").at(hi, k[1], "false"),
				)
			}
			for _, k := range rootPairs {
				sends = append(sends,
					tlsSend{}.at(lo, k[0], missingCA).at(hi, k[1], goodCA),
					tlsSend{want: errMissing}.at(lo, k[0], goodCA).at(hi, k[1], missingCA),
				)
			}
			// A layer may write both names when a higher layer sets the setting.
			off := tlsSend{}.at(lo, "insecure", "false").at(lo, "http-insecure", "false")
			on := tlsSend{want: errVerify}.at(lo, "insecure", "true").at(lo, "http-insecure", "true")
			for _, k := range []string{"insecure", "http-insecure"} {
				sends = append(sends, off.at(hi, k, "true"), on.at(hi, k, "false"))
			}
		}
	}
	for sc := range numScopes {
		for _, v := range [][2]string{{"true", "false"}, {"false", "true"}, {"true", "true"}} {
			sends = append(sends, tlsSend{want: errSameInsecure}.at(sc, "insecure", v[0]).at(sc, "http-insecure", v[1]))
		}
		for _, v := range [][2]string{{missingCA, goodCA}, {goodCA, missingCA}, {goodCA, goodCA}} {
			sends = append(
				sends,
				tlsSend{want: errSameRoots}.at(sc, "http-root-ca", v[0]).at(sc, "http-root-cas", v[1]),
			)
		}
		// An HTTP request never reads the gRPC keys, so their conflict is not its error.
		sends = append(sends, tlsSend{}.
			at(sc, "grpc-root-ca", missingCA).at(sc, "grpc-root-cas", missingCA).at(scopeRequest, "insecure", "true"))
	}
	sends = append(sends,
		tlsSend{want: `invalid http-insecure "maybe"`}.
			at(scopeEnv, "insecure", "true").at(scopeRequest, "http-insecure", "maybe"),
		tlsSend{want: `invalid insecure "maybe"`}.
			at(scopeEnv, "http-insecure", "true").at(scopeRequest, "insecure", "maybe"),
		tlsSend{want: errVerify}.at(scopeEnv, "http-insecure", "maybe").at(scopeRequest, "insecure", "false"),
		tlsSend{want: errVerify}.at(scopeEnv, "insecure", "maybe").at(scopeRequest, "http-insecure", "false"),
		tlsSend{want: errVerify}.at(scopeShared, "insecure", "maybe").at(scopeApply, "http-insecure", "false"),
		tlsSend{want: errVerify}.at(scopeEnv, "http-root-ca", missingCA).at(scopeRequest, "http-root-cas", ""),
		// An empty value still belongs to its layer, so it replaces the other name below it.
		tlsSend{want: "missing http-insecure value"}.
			at(scopeShared, "insecure", "true").at(scopeEnv, "http-insecure", ""),
		tlsSend{want: errVerify}.at(scopeShared, "http-root-ca", goodCA).at(scopeEnv, "http-root-cas", ""),
		// env: references resolve before the layers are compared.
		tlsSend{}.at(scopeShared, "insecure", "false").at(scopeEnv, "http-insecure", "env:"+on),
		tlsSend{want: errVerify}.at(scopeShared, "http-insecure", "env:"+on).at(scopeEnv, "insecure", "false"),
		// A reference to a missing variable leaves its name unset, so the other name below it applies.
		tlsSend{}.at(scopeShared, "insecure", "true").at(scopeEnv, "http-insecure", "env:"+unset),
		// The same name below it does not. The environment replaced it before the reference was read.
		tlsSend{want: errVerify}.at(scopeShared, "insecure", "true").at(scopeEnv, "insecure", "env:"+unset),
	)

	path := filepath.Join(dir, "tls.http")
	for _, s := range sends {
		t.Run(s.name, func(t *testing.T) {
			n := seen.Load()
			s.run(t, &restfile.Document{Path: path}, &restfile.Request{Method: http.MethodGet, URL: srv.URL})
			if s.want != "" && s.want != errVerify && seen.Load() != n {
				t.Fatal("request reached the server after a settings error")
			}
		})
	}
}

func TestExecuteTLSSettingNamesFollowGroupLayers(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	writeCA(t, dir, srv)

	group := func(name string, set map[string]string) vars.Group {
		vals := map[string]string{}
		for k, v := range set {
			vals["settings."+k] = v
		}
		return vars.Group{Name: name, Profiles: vars.EnvironmentSet{"dev": vals}}
	}
	profile := func(want, key, val string) tlsSend {
		return tlsSend{
			name:   fmt.Sprintf("profile %s=%s", key, val),
			groups: []vars.Group{group("api", map[string]string{key: val})},
			want:   want,
		}
	}
	var sends []tlsSend
	for _, k := range insecurePairs {
		sends = append(sends,
			profile("", k[1], "true").at(scopeShared, k[0], "false"),
			profile(errVerify, k[1], "false").at(scopeShared, k[0], "true"),
		)
	}
	for _, k := range rootPairs {
		sends = append(sends,
			profile("", k[1], goodCA).at(scopeShared, k[0], missingCA),
			profile(errMissing, k[1], missingCA).at(scopeShared, k[0], goodCA),
		)
	}
	// Profiles from all groups form one layer.
	two := []vars.Group{
		group("a", map[string]string{"insecure": "true"}),
		group("b", map[string]string{"http-insecure": "false"}),
	}
	both := profile(errVerify, "insecure", "false").at(scopeShared, "insecure", "true")
	sends = append(sends,
		both.at(scopeShared, "http-insecure", "true"),
		tlsSend{name: "two groups", groups: two, want: errSameInsecure},
		tlsSend{name: "two groups", groups: two, want: errVerify}.at(scopeRequest, "insecure", "false"),
		tlsSend{name: "two groups", groups: two}.at(scopeRequest, "http-insecure", "true"),
	)

	path := filepath.Join(dir, "tls.http")
	for _, s := range sends {
		t.Run(s.name, func(t *testing.T) {
			s.run(t, &restfile.Document{Path: path}, &restfile.Request{Method: http.MethodGet, URL: srv.URL})
		})
	}
}

func TestExecuteApplyOrderAndNull(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	writeCA(t, dir, srv)

	patches := []restfile.PatchProfile{
		{Scope: directive.ScopeFile, Name: "loose", Expression: `{settings: {"insecure": true}}`},
		{Scope: directive.ScopeFile, Name: "strict", Expression: `{settings: {"http-insecure": false}}`},
	}
	apply := func(exprs ...string) []restfile.ApplySpec {
		out := make([]restfile.ApplySpec, len(exprs))
		for i, ex := range exprs {
			out[i] = restfile.ApplySpec{Expression: ex}
		}
		return out
	}
	for _, tt := range []struct {
		name    string
		send    tlsSend
		applies []restfile.ApplySpec
	}{
		{"use loose then strict", tlsSend{want: errVerify}, []restfile.ApplySpec{{Uses: []string{"loose", "strict"}}}},
		{"use strict then loose", tlsSend{}, []restfile.ApplySpec{{Uses: []string{"strict", "loose"}}}},
		{
			"null brings back the env value",
			tlsSend{}.at(scopeEnv, "insecure", "true").at(scopeRequest, "http-insecure", "false"),
			apply(`{settings: {"insecure": null}}`),
		},
		{
			"null keeps the env value",
			tlsSend{want: errVerify}.at(scopeEnv, "insecure", "false").at(scopeRequest, "http-insecure", "true"),
			apply(`{settings: {"insecure": null}}`),
		},
		{
			"null on http-insecure keeps the env value",
			tlsSend{want: errVerify}.at(scopeEnv, "http-insecure", "false").at(scopeRequest, "insecure", "true"),
			apply(`{settings: {"http-insecure": null}}`),
		},
		{
			"null brings back the file value",
			tlsSend{}.at(scopeFile, "insecure", "true").at(scopeRequest, "http-insecure", "false"),
			apply(`{settings: {"insecure": null}}`),
		},
		{
			"null removes an earlier patch",
			tlsSend{}.at(scopeEnv, "insecure", "true"),
			apply(`{settings: {"http-insecure": false}}`, `{settings: {"insecure": null}}`),
		},
		{
			"null on http-root-ca brings back the env value",
			tlsSend{}.at(scopeEnv, "http-root-ca", goodCA).at(scopeRequest, "http-root-cas", missingCA),
			apply(`{settings: {"http-root-ca": null}}`),
		},
		{
			"null on http-root-cas keeps the env value",
			tlsSend{want: errMissing}.at(scopeEnv, "http-root-cas", missingCA).at(scopeRequest, "http-root-ca", goodCA),
			apply(`{settings: {"http-root-cas": null}}`),
		},
		{
			"null and false in one patch",
			tlsSend{want: errVerify},
			apply(`{settings: {"insecure": null, "http-insecure": false}}`),
		},
		{
			"null and true in one patch",
			tlsSend{},
			apply(`{settings: {"insecure": null, "http-insecure": true}}`),
		},
		{
			"null and false over the env value",
			tlsSend{want: errVerify}.at(scopeEnv, "insecure", "true"),
			apply(`{settings: {"http-insecure": null, "insecure": false}}`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			doc := &restfile.Document{Path: filepath.Join(dir, "tls.http"), Patches: patches}
			req := &restfile.Request{
				Method:   http.MethodGet,
				URL:      srv.URL,
				Metadata: restfile.RequestMetadata{Applies: tt.applies},
			}
			tt.send.run(t, doc, req)
		})
	}
}

func TestApplySettingsCaseFormsResolveInOrder(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	doc := &restfile.Document{}
	want := map[string]string{"insecure": "false"}
	// Map order is random, so one pass could pick the right form by luck.
	for range 100 {
		req := &restfile.Request{
			Method: http.MethodGet,
			URL:    "https://example.com",
			Metadata: restfile.RequestMetadata{Applies: []restfile.ApplySpec{{
				Expression: `{settings: {"Insecure": true, "insecure": false}}`,
			}}},
		}
		env := testEnv("").Resolve()
		if err := e.runRTSApply(
			context.Background(),
			doc,
			e.RunDoc(doc).File,
			req,
			env,
			"",
			evalScope{},
			rts.Locals{},
		); err != nil {
			t.Fatalf("runRTSApply() error = %v", err)
		}
		if !maps.Equal(req.Settings, want) {
			t.Fatalf("Settings = %v, want %v", req.Settings, want)
		}
	}
}

func TestExecuteTLSSettingNamesInStreamsAndOAuth(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sse":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: hi\n\n")
		case "/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			_, _, _ = conn.Read(r.Context())
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
		}
	}))
	t.Cleanup(srv.Close)
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(plain.Close)

	kinds := []struct {
		name string
		req  func() *restfile.Request
	}{
		{"sse", func() *restfile.Request {
			return &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/sse", SSE: &restfile.SSERequest{}}
		}},
		{"websocket", func() *restfile.Request {
			ws := &restfile.WebSocketRequest{Steps: []restfile.WebSocketStep{{Type: restfile.WebSocketStepClose}}}
			url := "wss" + strings.TrimPrefix(srv.URL, "https") + "/ws"
			return &restfile.Request{Method: http.MethodGet, URL: url, WebSocket: ws}
		}},
		{"oauth", func() *restfile.Request {
			auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
				"token_url":     srv.URL + "/token",
				"client_id":     "c",
				"client_secret": "s",
				"grant":         "client_credentials",
			}}
			return &restfile.Request{
				Method:   http.MethodGet,
				URL:      plain.URL,
				Metadata: restfile.RequestMetadata{Auth: auth},
			}
		}},
	}
	path := filepath.Join(t.TempDir(), "tls.http")
	for _, kind := range kinds {
		for _, sc := range [][2]scope{{scopeEnv, scopeRequest}, {scopeShared, scopeEnv}} {
			for _, k := range [][2]string{{"http-insecure", "insecure"}, {"insecure", "http-insecure"}} {
				for _, s := range []tlsSend{
					tlsSend{}.at(sc[0], k[0], "false").at(sc[1], k[1], "true"),
					tlsSend{want: errVerify}.at(sc[0], k[0], "true").at(sc[1], k[1], "false"),
				} {
					t.Run(kind.name+" "+s.name, func(t *testing.T) {
						s.run(t, &restfile.Document{Path: path}, kind.req())
					})
				}
			}
		}
	}
}

func TestPreviewShowsOneSettingName(t *testing.T) {
	for _, tt := range []struct {
		lo, hi        scope
		lower, higher string
	}{
		{scopeEnv, scopeRequest, "http-insecure", "insecure"},
		{scopeEnv, scopeRequest, "insecure", "http-insecure"},
		{scopeShared, scopeEnv, "http-insecure", "insecure"},
		{scopeShared, scopeEnv, "insecure", "http-insecure"},
		{scopeRequest, scopeApply, "http-insecure", "insecure"},
		{scopeRequest, scopeApply, "insecure", "http-insecure"},
	} {
		s := tlsSend{}.at(tt.lo, tt.lower, "true").at(tt.hi, tt.higher, "false")
		t.Run(s.name, func(t *testing.T) {
			req := &restfile.Request{Method: http.MethodGet, URL: "https://example.com"}
			res := s.exec(t, &restfile.Document{}, req, ExecOptions{Mode: ExecModePreview})
			if res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v", res.Err)
			}
			want := []xplain.Pair{{Key: tt.higher, Value: "false"}}
			if got := res.Explain.Final.Settings; !slices.Equal(got, want) {
				t.Fatalf("Final.Settings = %v, want %v", got, want)
			}
		})
	}
}
