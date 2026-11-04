package request

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/unkn0wn-root/resterm/internal/directive"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

func TestReportResponseCutsBodyAtLimit(t *testing.T) {
	const limit = runfmt.MaxBodyBytes
	tests := []struct {
		name string
		body string
		want int
		cut  bool
	}{
		{"under the limit", strings.Repeat("a", limit-1), limit - 1, false},
		{"exactly the limit", strings.Repeat("a", limit), limit, false},
		{"twice the limit", strings.Repeat("a", 2*limit), limit, true},
		{"split rune", strings.Repeat("a", limit-1) + "é" + "b", limit - 1, true},
		{"split rune before binary", strings.Repeat("a", limit-1) + "é\xff\xfe", limit - 1, true},
		{"four byte rune", strings.Repeat("a", limit-3) + "😀" + "b", limit - 3, true},
		{"binary", strings.Repeat("\x80", limit+8), limit, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportResponse(&httpx.Response{StatusCode: 200, Body: []byte(tt.body)}, nil, nil, nil)
			if len(got.Body) != tt.want || got.BodyTruncated != tt.cut {
				t.Fatalf("body is %d bytes, truncated %t, want %d bytes, truncated %t",
					len(got.Body), got.BodyTruncated, tt.want, tt.cut)
			}
			if got.BodySize != int64(len(tt.body)) {
				t.Fatalf("BodySize = %d, want the full size %d", got.BodySize, len(tt.body))
			}
			if string(got.Body) != tt.body[:tt.want] {
				t.Fatal("body is not a prefix of the response")
			}
			if utf8.ValidString(tt.body[:tt.want]) && !utf8.Valid(got.Body) {
				t.Fatal("the cut split a rune")
			}
		})
	}
}

func TestReportResponseMasksBeforeCut(t *testing.T) {
	const secret = "s3cr3t-tok"
	body := strings.Repeat("a", runfmt.MaxBodyBytes-3) + secret + strings.Repeat("b", 10)
	got := ReportResponse(&httpx.Response{StatusCode: 200, Body: []byte(body)}, nil, nil, []string{secret})
	if !got.BodyTruncated || got.BodySize != int64(len(body)) {
		t.Fatalf("truncated %t, BodySize %d, want a cut body of %d bytes", got.BodyTruncated, got.BodySize, len(body))
	}
	for i := 1; i <= len(secret); i++ {
		if bytes.HasSuffix(got.Body, []byte(secret[:i])) {
			t.Fatalf("body ends with %q, a prefix of the secret", secret[:i])
		}
	}
	if !bytes.HasSuffix(got.Body, []byte("•")) {
		t.Fatalf("body ends with %q, want the mask", got.Body[len(got.Body)-8:])
	}
}

func TestReportResponseMasksSensitiveHeaders(t *testing.T) {
	const secret = "s3cr3t-tok"
	tests := []struct {
		name string
		key  string
		vals []string
		want []string
	}{
		{"api key", "X-Api-Key", []string{"resp-key-123"}, []string{"•••"}},
		{"mixed case and two values", "x-API-key", []string{"k1", "k2"}, []string{"•••", "•••"}},
		{"authorization", "Authorization", []string{"Bearer t"}, []string{"•••"}},
		{"set-cookie", "Set-Cookie", []string{"sid=c1; Path=/", "b=2"}, []string{"•••", "•••"}},
		{"set-cookie2", "set-cookie2", []string{"old=1"}, []string{"•••"}},
		{"plain header", "X-Request-Id", []string{"req-1"}, []string{"req-1"}},
		{"secret in a plain header", "Location", []string{"/next?token=" + secret}, []string{"/next?token=•••"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &httpx.Response{StatusCode: 200, Headers: http.Header{tt.key: tt.vals}}
			got := ReportResponse(resp, nil, nil, []string{secret})
			want := http.Header{tt.key: tt.want}
			if !reflect.DeepEqual(got.Headers, want) {
				t.Fatalf("Headers = %v, want %v", got.Headers, want)
			}
		})
	}
}

func TestReportResponseMasksCaptures(t *testing.T) {
	const secret = "s3cr3t-tok"
	caps := []restfile.Variable{
		{Name: "tok", Value: secret, Secret: true, Scope: directive.ScopeRequest},
		{Name: "id", Value: "42", Scope: directive.ScopeFile},
		{Name: "next", Value: "/next?token=" + secret, Scope: directive.ScopeGlobal},
	}
	got := ReportResponse(&httpx.Response{StatusCode: 204}, nil, caps, []string{secret})
	want := []runfmt.Capture{
		{Scope: "request", Name: "tok", Value: "•••", Secret: true},
		{Scope: "file", Name: "id", Value: "42"},
		{Scope: "global", Name: "next", Value: "/next?token=•••"},
	}
	if !reflect.DeepEqual(got.Captures, want) {
		t.Fatalf("Captures = %+v, want %+v", got.Captures, want)
	}
}

func TestReportResponseStreamBody(t *testing.T) {
	raw := `{"events":[{"data":"hi"}]}`
	resp := &httpx.Response{
		StatusCode: 200,
		Headers: http.Header{
			"Content-Type":            {"application/json"},
			httpx.StreamHeaderType:    {"sse"},
			httpx.StreamHeaderSummary: {"events=1"},
		},
		Body: []byte(raw),
	}
	got := ReportResponse(resp, nil, nil, nil)
	if string(got.Body) != raw {
		t.Fatalf("body = %q, want the transcript %q", got.Body, raw)
	}
	if !reflect.DeepEqual(got.Headers, resp.Headers) {
		t.Fatalf("Headers = %v, want the stream headers kept", got.Headers)
	}
}

func TestReportResponseNilWhenNothingArrived(t *testing.T) {
	tests := []struct {
		name string
		resp *httpx.Response
	}{
		{name: "no result"},
		{name: "http zero value", resp: &httpx.Response{}},
		{name: "http dial error", resp: &httpx.Response{Duration: time.Millisecond}},
	}
	caps := []restfile.Variable{{Name: "id", Value: "42"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReportResponse(tt.resp, nil, caps, nil); got != nil {
				t.Fatalf("ReportResponse(...) = %+v, want nil", got)
			}
		})
	}
}

func TestExecuteReturnsCapturesInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"token":"t1","id":42}`)
	}))
	defer srv.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(gs, emptySvc{})
	reflection.Register(gs)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	tests := []struct {
		name string
		src  string
		want []restfile.Variable
	}{
		{
			name: "http",
			src: "### one\n" +
				"# @capture file-secret tok {{response.json.token}}\n" +
				"# @capture request id {{response.json.id}}\n" +
				"# @capture global again {{response.json.id}}\n" +
				"GET " + srv.URL + "\n",
			want: []restfile.Variable{
				{Name: "tok", Value: "t1", Secret: true, Scope: directive.ScopeFile},
				{Name: "id", Value: "42", Scope: directive.ScopeRequest},
				{Name: "again", Value: "42", Scope: directive.ScopeGlobal},
			},
		},
		{
			name: "grpc",
			src: "### one\n# @grpc grpc.testing.TestService/EmptyCall\n# @grpc-plaintext true\n" +
				"# @capture request st {{response.status}}\n" +
				"GRPC " + lis.Addr().String() + "\n\n{}\n",
			want: []restfile.Variable{{Name: "st", Value: "OK", Scope: directive.ScopeRequest}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, req := parseDoc(t, tt.src)
			res, err := New(engcfg.Config{}, nil).ExecuteWith(doc, req, testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() = %v, %v", res.Err, err)
			}
			if !reflect.DeepEqual(res.Captures, tt.want) {
				t.Fatalf("Captures = %+v, want %+v", res.Captures, tt.want)
			}
		})
	}
}

func TestReportResponseDecodesContentCoding(t *testing.T) {
	const secret = "s3cr3t-tok"
	pad := strings.Repeat("ab", 100)
	plain := []byte(`{"token":"` + secret + `","pad":"` + pad + `"}`)
	masked := `{"token":"•••","pad":"` + pad + `"}`
	if bytes.Contains(gzipBytes(t, plain), []byte(secret)) {
		t.Fatal("the gzip body holds the secret in clear")
	}
	var deflated bytes.Buffer
	fw, err := flate.NewWriter(&deflated, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := fw.Write(plain); err != nil || fw.Close() != nil {
		t.Fatalf("deflate: %v", err)
	}
	bomb := gzipBytes(t, make([]byte, httpx.DefaultMaxResponseBytes+1))

	tests := []struct {
		name    string
		enc     []string
		body    []byte
		want    string
		decoded bool
	}{
		{"gzip", []string{"gzip"}, gzipBytes(t, plain), masked, true},
		{"gzip in capitals", []string{"GZIP"}, gzipBytes(t, plain), masked, true},
		{"identity", []string{"identity"}, plain, masked, false},
		{"deflate", []string{"deflate"}, deflated.Bytes(), "", false},
		{"br", []string{"br"}, append([]byte{0x8b, 0x02, 0x80}, plain...), "", false},
		{"invalid gzip", []string{"gzip"}, plain, "", false},
		{"gzip over the decode limit", []string{"gzip"}, bomb, "", false},
		{"identity then gzip", []string{"identity", "gzip"}, gzipBytes(t, plain), "", false},
	}
	caps := []restfile.Variable{{Name: "tok", Value: secret, Secret: true, Scope: directive.ScopeRequest}}
	wantCaps := []runfmt.Capture{{Scope: "request", Name: "tok", Value: "•••", Secret: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := strconv.Itoa(len(tt.body))
			resp := &httpx.Response{
				StatusCode: 200,
				Headers: http.Header{
					"Content-Type":     {"application/json"},
					"Content-Encoding": tt.enc,
					"Content-Length":   {size},
				},
				Body: tt.body,
			}
			got := ReportResponse(resp, nil, caps, []string{secret})
			if string(got.Body) != tt.want {
				t.Fatalf("body = %.80q, want %q", got.Body, tt.want)
			}
			if got.BodySize != int64(len(tt.body)) {
				t.Fatalf("BodySize = %d, want the received size %d", got.BodySize, len(tt.body))
			}
			want := http.Header{"Content-Type": {"application/json"}}
			if !tt.decoded {
				want["Content-Encoding"] = tt.enc
				want["Content-Length"] = []string{size}
			}
			if !reflect.DeepEqual(got.Headers, want) {
				t.Fatalf("Headers = %q, want %q", got.Headers, want)
			}
			if !reflect.DeepEqual(got.Captures, wantCaps) {
				t.Fatalf("Captures = %+v, want %+v", got.Captures, wantCaps)
			}
		})
	}
}

func TestReportResponseMasksGzipBeforeCut(t *testing.T) {
	const secret = "s3cr3t-tok"
	plain := strings.Repeat("a", runfmt.MaxBodyBytes-3) + secret + strings.Repeat("b", 10)
	resp := &httpx.Response{
		StatusCode: 200,
		Headers:    http.Header{"Content-Encoding": {"gzip"}},
		Body:       gzipBytes(t, []byte(plain)),
	}
	got := ReportResponse(resp, nil, nil, []string{secret})
	if !got.BodyTruncated || len(got.Body) != runfmt.MaxBodyBytes {
		t.Fatalf("body is %d bytes, truncated %t, want the decoded body cut to %d bytes",
			len(got.Body), got.BodyTruncated, runfmt.MaxBodyBytes)
	}
	for i := 1; i <= len(secret); i++ {
		if bytes.HasSuffix(got.Body, []byte(secret[:i])) {
			t.Fatalf("body ends with %q, a prefix of the secret", secret[:i])
		}
	}
}

func TestReportResponseMasksBinaryBody(t *testing.T) {
	const secret = "s3cr3t-tok"
	resp := &httpx.Response{StatusCode: 200, Body: []byte("\xff\xfe" + secret + "\x00\x01")}
	got := ReportResponse(resp, nil, nil, []string{secret})
	if want := "\xff\xfe•••\x00\x01"; string(got.Body) != want {
		t.Fatalf("body = %q, want %q", got.Body, want)
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if _, err := z.Write(data); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if err := z.Close(); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	return buf.Bytes()
}

type reportSvc struct {
	testgrpc.UnimplementedTestServiceServer
}

func (reportSvc) EmptyCall(context.Context, *testgrpc.Empty) (*testgrpc.Empty, error) {
	return &testgrpc.Empty{}, nil
}

func (reportSvc) UnaryCall(ctx context.Context, req *testgrpc.SimpleRequest) (*testgrpc.SimpleResponse, error) {
	if st := req.GetResponseStatus(); st != nil {
		return nil, status.Error(codes.Code(st.GetCode()), st.GetMessage())
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs("x-api-key", "k1")); err != nil {
		return nil, err
	}
	if err := grpc.SetTrailer(ctx, metadata.Pairs("x-done", "yes")); err != nil {
		return nil, err
	}
	return &testgrpc.SimpleResponse{Username: "s3cr3t-tok"}, nil
}

func (reportSvc) StreamingOutputCall(
	_ *testgrpc.StreamingOutputCallRequest,
	s grpc.ServerStreamingServer[testgrpc.StreamingOutputCallResponse],
) error {
	for range 2 {
		if err := s.Send(&testgrpc.StreamingOutputCallResponse{}); err != nil {
			return err
		}
	}
	return nil
}

func TestReportResponseGRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(gs, reportSvc{})
	reflection.Register(gs)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	dir := t.TempDir()
	proto := "syntax = \"proto3\";\npackage demo;\nmessage E {}\nservice S { rpc Call(E) returns (E); }\n"
	if err := os.WriteFile(filepath.Join(dir, "s.proto"), []byte(proto), 0o644); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	call := func(method, extra, body string) string {
		return "### one\n# @grpc " + method + "\n# @grpc-plaintext true\n" + extra +
			"GRPC " + lis.Addr().String() + "\n\n" + body + "\n"
	}
	tests := []struct {
		name string
		src  string
		hdr  http.Header
		caps []runfmt.Capture
	}{
		{
			name: "ok without metadata",
			src:  call("grpc.testing.TestService/EmptyCall", "", "{}"),
			hdr:  http.Header{"content-type": {"application/grpc"}},
		},
		{
			name: "metadata and a secret",
			src: call("grpc.testing.TestService/UnaryCall",
				"# @capture request-secret user {{response.json.username}}\n", "{}"),
			hdr: http.Header{
				"content-type":        {"application/grpc"},
				"x-api-key":           {"•••"},
				"Grpc-Trailer-x-done": {"yes"},
			},
			caps: []runfmt.Capture{{Scope: "request", Name: "user", Value: "•••", Secret: true}},
		},
		{
			name: "error status",
			src:  call("grpc.testing.TestService/UnaryCall", "", `{"responseStatus":{"code":7,"message":"no"}}`),
			hdr: http.Header{
				"Grpc-Trailer-content-type": {"application/grpc"},
				"Content-Type":              {"application/json"},
			},
		},
		{
			name: "stream",
			src:  call("grpc.testing.TestService/StreamingOutputCall", "", "{}"),
			hdr:  http.Header{"content-type": {"application/grpc"}},
		},
		{
			name: "dial error",
			src: "### one\n# @grpc demo.S/Call\n# @grpc-plaintext true\n# @grpc-proto ./s.proto\n" +
				"GRPC " + closed.Addr().String() + "\n\n{}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, "grpc.http")
			doc := parser.Parse(path, []byte(tt.src))
			res, err := New(
				engcfg.Config{FilePath: path},
				nil,
			).ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.GRPC == nil {
				t.Fatalf("no gRPC result: %v", res.Err)
			}
			var want *runfmt.Response
			if tt.hdr != nil {
				view, _ := res.GRPC.ViewBody()
				want = &runfmt.Response{
					Headers:  tt.hdr,
					Body:     []byte(strings.ReplaceAll(string(view), "s3cr3t-tok", "•••")),
					BodySize: int64(len(view)),
					Captures: tt.caps,
				}
			}
			got := ReportResponse(nil, res.GRPC, res.Captures, res.RuntimeSecrets)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ReportResponse(...) = %+v\nwant %+v", got, want)
			}
		})
	}
}
