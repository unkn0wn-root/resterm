package runview

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/coder/websocket"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"google.golang.org/grpc/codes"
)

func TestRenderHTTPPrettyWithHeadersAndFailures(t *testing.T) {
	rep := testHTTPPrettyReport()

	out, err := Render(rep, Options{Mode: ModePretty, Headers: true})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected plain output, got %q", out)
	}
	for _, want := range []string{
		"Name: items",
		"Request: GET https://example.com/items",
		"Environment: dev",
		"Status: 200 OK",
		"Duration: 12ms",
		"Request Headers:",
		"Response Headers:",
		"Errors:",
		"Script error:",
		"error[unknown]: script boom",
		"Warnings:",
		"Unresolved template variables: reporting.token",
		"Tests:",
		"[PASS] method - expected GET (1ms)",
		"[FAIL] status - expected 201 (3ms)",
		`id: 1`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRenderPrettyAlignsTestMessageLines(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "orders",
			Method: "GET",
			Target: "https://example.com/orders",
			Tests: []scriptapi.TestResult{{
				Name:    "orders",
				Message: "api.http:11: Order 0 has negative total\napi.http:11: Order 3 missing tracking",
				Elapsed: 3 * time.Millisecond,
			}},
		}},
	}

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	want := "Tests:\n" +
		"  [FAIL] orders - api.http:11: Order 0 has negative total\n" +
		"                  api.http:11: Order 3 missing tracking (3ms)"
	if !strings.Contains(out, want) {
		t.Fatalf("output =\n%s\nwant it to contain\n%s", out, want)
	}
}

func TestRenderPrettyLeavesBlankLinesEmpty(t *testing.T) {
	rep := diag.Report{
		Path:   "two.http",
		Source: []byte(`# @assert fail("a") ` + "\n"),
		Items: []diag.Diagnostic{
			{
				Class:   diag.ClassScript,
				Message: "a",
				Span:    diag.Span{Start: diag.Pos{Path: "two.http", Line: 1, Col: 11}},
			},
			{Class: diag.ClassScript, Message: "b"},
		},
	}
	res := runner.Result{
		Kind:      runner.ResultKindRequest,
		Name:      "two",
		Method:    "GET",
		Target:    "https://example.com/two",
		ScriptErr: diag.FromReport(rep, errors.New("a")),
		Tests:     []scriptapi.TestResult{{Name: "t", Message: "a\n\nb"}},
	}

	out, err := Render(&runner.Report{Results: []runner.Result{res}}, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	want := "Errors:\n" +
		"  Script error:\n" +
		"    error[script]: a\n" +
		"    --> two.http:1:11\n" +
		"         |\n" +
		`       1 | # @assert fail("a") ` + "\n" +
		"         |           ^\n" +
		"\n" +
		"    error[script]: b\n" +
		"    --> two.http\n" +
		"\n" +
		"Tests:\n" +
		"  [FAIL] t - a\n" +
		"\n" +
		"             b"
	if !strings.Contains(out, want) {
		t.Fatalf("output =\n%q\nwant it to contain\n%q", out, want)
	}
}

func TestRenderPrettyEscapesTestText(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "orders",
			Method: "GET",
			Target: "https://example.com/orders",
			Tests: []scriptapi.TestResult{{
				Name:    "orders\x1b]0;pwn\a",
				Message: "api.http:3: got \x1b[31mred\r\nnext\tline",
			}},
		}},
	}

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if strings.ContainsAny(out, "\x1b\a\r\t") {
		t.Fatalf("output has raw control characters: %q", out)
	}
}

func TestRenderHTTPPrettyUsesTrackedUnresolvedVarsAndEffectiveTarget(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:            runner.ResultKindRequest,
			Name:            "ReportsList",
			Method:          "GET",
			Target:          "{{services.api.base}}/reports",
			EffectiveTarget: "https://httpbin.org/anything/api/reports",
			Environment:     "dev",
			Response: &httpx.Response{
				Status:       "200 OK",
				StatusCode:   200,
				Headers:      http.Header{"Content-Type": {"application/json"}},
				Body:         []byte(`{"ok":true}`),
				Duration:     463 * time.Millisecond,
				EffectiveURL: "https://httpbin.org/anything/api/reports",
				ReqMethod:    "GET",
			},
		}},
	}
	rep.Results[0].SetUnresolvedTemplateVars([]string{"reporting.token"})

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "Request: GET https://httpbin.org/anything/api/reports") {
		t.Fatalf("expected effective request target, got %q", out)
	}
	if !strings.Contains(out, "Source Target: {{services.api.base}}/reports") {
		t.Fatalf("expected source target details, got %q", out)
	}
	if !strings.Contains(out, "Unresolved template variables: reporting.token") {
		t.Fatalf("expected tracked unresolved variable warning, got %q", out)
	}
	for _, bad := range []string{
		"reporting.apiKey",
		"reporting.sharedSecret",
	} {
		if strings.Contains(out, bad) {
			t.Fatalf("did not expect %q in output, got %q", bad, out)
		}
	}
}

func TestRenderHTTPPrettyColorPreservesPlainText(t *testing.T) {
	rep := testHTTPPrettyReport()

	plain, err := Render(rep, Options{Mode: ModePretty, Headers: true})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	out, err := Render(rep, Options{
		Mode:    ModePretty,
		Headers: true,
		Color:   termcolor.TrueColor(),
	})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected colored output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain {
		t.Fatalf("expected color stripping to preserve output\nwant:\n%s\n\ngot:\n%s", plain, got)
	}
}

func TestRenderGRPCRawFallsBackToStreamSummary(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Method: "GRPC",
			Target: "/demo.Service/Watch",
			GRPC: &grpcx.Response{
				StatusCode:    codes.OK,
				StatusMessage: "ok",
				Duration:      25 * time.Millisecond,
			},
			Stream: &runner.StreamInfo{
				Kind: "websocket",
				Summary: map[string]any{
					"eventCount": 2,
					"reason":     "complete",
				},
			},
		}},
	}

	out, err := Render(rep, Options{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	for _, want := range []string{
		"Request: GRPC /demo.Service/Watch",
		"Status: OK",
		"Raw Body:",
		"Stream: websocket",
		"eventCount: 2",
		"reason: complete",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRenderFallsBackToReportTextForWorkflow(t *testing.T) {
	rep := &runner.Report{
		Total:  1,
		Passed: 1,
		Results: []runner.Result{{
			Kind:     runner.ResultKindWorkflow,
			Name:     "deploy",
			Method:   "WORKFLOW",
			Duration: time.Second,
			Passed:   true,
			Steps: []runner.StepResult{{
				Name:     "stage",
				Passed:   true,
				Duration: 250 * time.Millisecond,
			}},
		}},
	}

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "PASS WORKFLOW deploy") {
		t.Fatalf("expected report text fallback, got %q", out)
	}
}

func TestRenderBodyRawReturnsBodyOnly(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "items",
			Method: "GET",
			Target: "https://example.com/items",
			Response: &httpx.Response{
				Status:       "200 OK",
				StatusCode:   200,
				Headers:      http.Header{"Content-Type": {"application/json"}},
				Body:         []byte(`{"id":1}`),
				Duration:     12 * time.Millisecond,
				EffectiveURL: "https://example.com/items",
				ReqMethod:    "GET",
			},
			Passed: true,
		}},
	}

	out, err := RenderBody(rep, BodyOptions{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if out != "{\n  \"id\": 1\n}" {
		t.Fatalf("unexpected body output %q", out)
	}
}

func TestRenderBodyPrettyCanColorOutput(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind: runner.ResultKindRequest,
			Response: &httpx.Response{
				Headers: http.Header{"Content-Type": {"application/json"}},
				Body:    []byte(`{"id":1}`),
			},
		}},
	}

	plain, err := RenderBody(rep, BodyOptions{Mode: ModePretty})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	out, err := RenderBody(rep, BodyOptions{Mode: ModePretty, Color: termcolor.TrueColor()})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected colored body output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain {
		t.Fatalf("expected stripped body to match plain output\nwant:\n%s\n\ngot:\n%s", plain, got)
	}
}

func TestRenderBodyPrettyUsesThemeSyntaxStyle(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind: runner.ResultKindRequest,
			Response: &httpx.Response{
				Headers: http.Header{"Content-Type": {"application/json"}},
				Body:    []byte(`{"id":1,"name":"demo"}`),
			},
		}},
	}

	dark, err := RenderBody(rep, BodyOptions{Mode: ModePretty, Color: termcolor.TrueColor()})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	light, err := RenderBody(rep, BodyOptions{
		Mode:  ModePretty,
		Color: termcolor.TrueColor(),
		Theme: &theme.Definition{
			Key: "daybreak",
			Metadata: theme.Metadata{
				Name: "Daybreak",
				Tags: []string{"light"},
			},
			Theme: theme.DefaultTheme(),
		},
	})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if dark == light {
		t.Fatalf("expected light theme syntax styling to differ from default dark styling")
	}
	if ansi.Strip(dark) != ansi.Strip(light) {
		t.Fatalf("expected syntax style changes to preserve text output")
	}
}

func testHTTPPrettyReport() *runner.Report {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:        runner.ResultKindRequest,
			Name:        "items",
			Method:      "GET",
			Target:      "https://example.com/items",
			Environment: "dev",
			Response: &httpx.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Headers:    http.Header{"Content-Type": {"application/json"}, "X-Resp": {"ok"}},
				RequestHeaders: http.Header{
					"Accept": {"application/json"},
				},
				ReqHost:      "example.com",
				Body:         []byte(`{"id":1}`),
				Duration:     12 * time.Millisecond,
				EffectiveURL: "https://example.com/items",
				ReqMethod:    "GET",
				ReqLen:       7,
				ReqTE:        []string{"gzip"},
			},
			ScriptErr: errors.New("script boom"),
			Tests: []scriptapi.TestResult{
				{
					Name:    "method",
					Message: "expected GET",
					Passed:  true,
					Elapsed: 1 * time.Millisecond,
				},
				{
					Name:    "status",
					Message: "expected 201",
					Passed:  false,
					Elapsed: 3 * time.Millisecond,
				},
			},
		}},
	}
	rep.Results[0].SetUnresolvedTemplateVars([]string{"reporting.token"})
	return rep
}

func TestRenderBodyKeepsDataWhileRenderEscapes(t *testing.T) {
	const body = "name\tvalue\nrow\tsoft\u00adcell"
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "rows",
			Method: "GET",
			Target: "https://example.com/rows",
			Response: &httpx.Response{
				Status:       "200 OK",
				StatusCode:   200,
				Headers:      http.Header{"Content-Type": {"text/tab-separated-values"}},
				Body:         []byte(body),
				Duration:     time.Millisecond,
				EffectiveURL: "https://example.com/rows",
				ReqMethod:    "GET",
			},
			Passed: true,
		}},
	}

	out, err := RenderBody(rep, BodyOptions{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if out != body {
		t.Errorf("--body changed the response: %q, want %q", out, body)
	}

	view, err := Render(rep, Options{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if strings.ContainsRune(view, '\u00ad') {
		t.Error("human output kept an invisible rune")
	}
	if !strings.Contains(view, `\u00ad`) {
		t.Error("human output dropped the escape")
	}
}

func TestRenderHTTPPrettyIncludesResultWarnings(t *testing.T) {
	rep := testHTTPPrettyReport()
	rep.Results[0].Warnings = []string{"Script sends {{token}} in header X-Id as written."}

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "Script sends {{token}} in header X-Id as written.") {
		t.Fatalf("expected the result warning, got %q", out)
	}
}

type reply struct {
	status string
	header http.Header
	body   string
	err    error
}

func runReport(t *testing.T, src string, sel runner.Select, replies map[string]reply) *runner.Report {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "view.http")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	client := httpx.NewClientWithOptions(httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
		return &http.Client{
			Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
				r := replies[req.URL.Path]
				if r.err != nil {
					return nil, r.err
				}
				code, _, _ := strings.Cut(r.status, " ")
				status, _ := strconv.Atoi(code)
				hdr := r.header.Clone()
				if hdr == nil {
					hdr = make(http.Header)
				}
				return &http.Response{
					Status:     r.status,
					StatusCode: status,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(r.body)),
					Request:    req,
				}, nil
			}),
		}, nil
	}))
	rep, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        sel,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rep
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

const viewSource = `### A
# @name login rv-s3c
# @capture rv-s3c token $.token
# @script pre-request
> request.setHeader("X-Note", "{{rv-s3c}}");
# @script test
> tests.assert(false, "saw rv-s3c");
# @script test
> throw new Error("bad rv-s3c");
GET https://api.example.com/start?k=rv-s3c
Authorization: Bearer rv-s3c
X-Custom: rv-s3c

### B
# @name other
# @request-secret tok rv-s3c
GET https://api.example.com/other
`

func TestRenderRequestMasksRequestData(t *testing.T) {
	rep := runReport(t, viewSource, runner.Select{Request: "login rv-s3c"}, map[string]reply{
		"/start": {status: "302 Found", header: http.Header{"Location": {"/final?k=rv-s3c"}}},
		"/final": {status: "200 OK", header: http.Header{"Content-Type": {"application/json"}}, body: "{}"},
	})
	for _, mode := range []Mode{ModePretty, ModeRaw} {
		out, err := Render(rep, Options{Mode: mode, Headers: true})
		if err != nil {
			t.Fatalf("Render(%s): %v", mode, err)
		}
		var lost []string
		for _, want := range []string{
			"Name: login •••",
			"Request: GET https://api.example.com/final?k=•••",
			"Source Target: https://api.example.com/start?k=•••",
			"Authorization: •••",
			"X-Custom: •••",
			"Referer: https://api.example.com/start?k=•••",
			"bad •••",
			"saw •••",
			`@capture scope "•••"`,
			"{{•••}}",
		} {
			if !strings.Contains(out, want) {
				lost = append(lost, want)
			}
		}
		if strings.Contains(out, "rv-s3c") || len(lost) > 0 {
			t.Errorf("%s view shows the secret or lacks %q:\n%s", mode, lost, out)
		}
	}
}

func TestRenderRequestKeepsResponse(t *testing.T) {
	const body = `{"token":"rv-s3c"}`
	rep := runReport(t, viewSource, runner.Select{Request: "login rv-s3c"}, map[string]reply{
		"/start": {
			status: "200 rv-s3c",
			header: http.Header{"Content-Type": {"application/json"}, "X-Echo": {"rv-s3c"}},
			body:   body,
		},
	})
	for _, mode := range []Mode{ModePretty, ModeRaw} {
		out, err := Render(rep, Options{Mode: mode, Headers: true})
		if err != nil {
			t.Fatalf("Render(%s): %v", mode, err)
		}
		for _, want := range []string{"Status: 200 rv-s3c", "X-Echo: rv-s3c", "rv-s3c\""} {
			if !strings.Contains(out, want) {
				t.Errorf("%s view lacks %q:\n%s", mode, want, out)
			}
		}
	}
}

func TestRenderRequestShowsTheDecodedGzipLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, `{"zip":"decoded"}`)
		_ = zw.Close()
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	file := filepath.Join(dir, "gzip.http")
	if err := os.WriteFile(file, []byte("GET "+srv.URL+"\nAccept-Encoding: gzip\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rep, err := runner.RunContext(context.Background(), runner.Options{FilePath: file, WorkspaceRoot: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Content-Length: 17 bytes") {
		t.Fatalf("view lacks the decoded length:\n%s", out)
	}
}

func TestRenderBodyKeepsResponse(t *testing.T) {
	const body = `{"token":"rv-s3c","note":"rv\/s3c"}`
	rep := runReport(t, viewSource, runner.Select{Request: "login rv-s3c"}, map[string]reply{
		"/start": {status: "200 OK", header: http.Header{"Content-Type": {"text/plain"}}, body: body},
	})
	out, err := RenderBody(rep, BodyOptions{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	if out != body {
		t.Errorf("--body = %q, want the response %q", out, body)
	}
}

var errDenied = errors.New("denied later-x9")

func TestReportWritersKeepReportRaw(t *testing.T) {
	src := `### R1
# @name r1
# @request-secret tok later-x9
GET https://api.example.com/one?k=later-x9

### R2
# @name r2
GET https://api.example.com/two

### R3
# @name r3
# @capture file-secret tok {{response.json.token}}
GET https://api.example.com/three
`
	replies := map[string]reply{
		"/one": {status: "200 OK", body: "{}"},
		"/two": {err: errDenied},
		"/three": {
			status: "200 OK",
			header: http.Header{"Content-Type": {"application/json"}},
			body:   `{"token":"later-x9"}`,
		},
	}
	for name, sel := range map[string]runner.Select{"all": {All: true}, "single": {Request: "r2"}} {
		t.Run(name, func(t *testing.T) {
			rep := runReport(t, src, sel, replies)
			errs := make([]error, len(rep.Results))
			for i, res := range rep.Results {
				errs[i] = res.Err
			}
			var outs []string
			for _, write := range []func(io.Writer) error{rep.WriteJSON, rep.WriteJUnit, rep.WriteText} {
				var b strings.Builder
				if err := write(&b); err != nil {
					t.Fatalf("write: %v", err)
				}
				outs = append(outs, b.String())
			}
			if CanRenderRequest(rep) {
				view, err := Render(rep, Options{Mode: ModePretty, Headers: true})
				if err != nil {
					t.Fatalf("Render: %v", err)
				}
				outs = append(outs, view)
			}
			for _, out := range outs {
				if strings.Contains(out, "later-x9") {
					t.Errorf("output shows the secret:\n%s", out)
				}
			}
			for i, res := range rep.Results {
				if res.Err != errs[i] {
					t.Errorf("result %d error changed to %v", i, res.Err)
				}
			}
			for _, res := range rep.Results {
				switch res.Name {
				case "r1":
					if !strings.Contains(res.EffectiveTarget, "later-x9") || !strings.Contains(res.Target, "later-x9") {
						t.Errorf("r1 targets %q and %q, want them raw", res.Target, res.EffectiveTarget)
					}
				case "r2":
					if !errors.Is(res.Err, errDenied) || !strings.Contains(res.Err.Error(), "later-x9") {
						t.Errorf("r2 error = %v, want the raw cause", res.Err)
					}
				}
			}
		})
	}
}

func TestRenderRequestMasksSentFrames(t *testing.T) {
	const sec = "ws-s3nt-77"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		for range 2 {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			_ = conn.Write(r.Context(), typ, append([]byte("echo "), data...))
		}
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}))
	defer srv.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "ws.http")
	src := "### ws\n# @name ws\n# @request-secret tok " + sec + "\n# @websocket timeout=5s\n" +
		"# @ws send {\"auth\":\"{{tok}}\"}\n" +
		"# @ws send-base64 " + base64.StdEncoding.EncodeToString([]byte("bin "+sec)) + "\n" +
		"GET " + strings.Replace(srv.URL, "http", "ws", 1) + "/ws\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rep, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Select:        runner.Select{Request: "ws"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	frames := func(t *testing.T, body string) map[string][]string {
		t.Helper()
		var tr httpx.WebSocketTranscript
		if err := json.Unmarshal([]byte(body), &tr); err != nil {
			t.Fatalf("body is not a transcript: %v\n%s", err, body)
		}
		out := make(map[string][]string)
		for _, ev := range tr.Events {
			if ev.Text == "" && ev.Base64 == "" {
				continue
			}
			frame := ev.Text
			if b, err := base64.StdEncoding.DecodeString(ev.Base64); err == nil && ev.Base64 != "" {
				frame = string(b)
			}
			out[ev.Direction] = append(out[ev.Direction], frame+" "+ev.Base64)
		}
		return out
	}
	out, err := Render(rep, Options{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	_, body, _ := strings.Cut(out, "Raw Body:\n")
	got := frames(t, body)
	if len(got["send"]) != 2 || len(got["receive"]) != 2 {
		t.Fatalf("frames = %q, want two sent and two received", got)
	}
	for _, f := range got["send"] {
		if strings.Contains(f, sec) || !strings.Contains(f, "•••") {
			t.Errorf("raw view shows the sent frame %q", f)
		}
	}
	for _, f := range got["receive"] {
		if !strings.Contains(f, "echo ") || !strings.Contains(f, sec) {
			t.Errorf("raw view changed the received frame %q", f)
		}
	}
	out, err = Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	echo := base64.StdEncoding.EncodeToString([]byte("echo bin " + sec))
	if strings.Count(out, sec) != 1 || !strings.Contains(out, `echo {\"auth\":\"`+sec) ||
		!strings.Contains(out, echo) ||
		strings.Contains(out, base64.StdEncoding.EncodeToString([]byte("bin "+sec))) {
		t.Errorf("pretty view shows a sent frame or changed a received one:\n%s", out)
	}
	raw, err := RenderBody(rep, BodyOptions{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("RenderBody: %v", err)
	}
	for _, f := range frames(t, raw)["send"] {
		if !strings.Contains(f, sec) {
			t.Errorf("--body changed the sent frame %q", f)
		}
	}
}

func TestRenderRequestShowsShortSecretWarning(t *testing.T) {
	src := "# @file-secret data a\n\n### A\nGET https://api.example.com/x\n"
	rep := runReport(t, src, runner.Select{All: true}, map[string]reply{"/x": {status: "200 OK", body: "{}"}})
	const want = `secret "data" is shorter than 4 characters, so reports may also mask the same word in unrelated text`
	for _, mode := range []Mode{ModePretty, ModeRaw} {
		out, err := Render(rep, Options{Mode: mode})
		if err != nil {
			t.Fatalf("Render(%s): %v", mode, err)
		}
		if !strings.Contains(out, want) {
			t.Errorf("%s view lacks %q:\n%s", mode, want, out)
		}
	}
}
