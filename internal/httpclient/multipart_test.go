package httpclient_test

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/curl/importer"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type recordedRequest struct {
	body []byte
	ct   string
}

func newRecordServer(t *testing.T) (*httptest.Server, *recordedRequest) {
	t.Helper()
	rec := &recordedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rec.body = data
		rec.ct = r.Header.Get("Content-Type")
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func (r *recordedRequest) boundary(t *testing.T) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.ct)
	if err != nil {
		t.Fatalf("parse content type %q: %v", r.ct, err)
	}
	return params["boundary"]
}

func (r *recordedRequest) form(t *testing.T) (map[string]string, map[string][]byte) {
	t.Helper()
	mr := multipart.NewReader(bytes.NewReader(r.body), r.boundary(t))
	values := map[string]string{}
	files := map[string][]byte{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return values, files
		}
		if err != nil {
			t.Fatalf("parse multipart body: %v\nwire: %q", err, r.body)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		if part.FileName() != "" {
			files[part.FormName()] = data
		} else {
			values[part.FormName()] = string(data)
		}
	}
}

func executeRequest(t *testing.T, req *restfile.Request, resolver *vars.Resolver, baseDir string) {
	t.Helper()
	client := httpclient.NewClient(httpclient.OSFileSystem{})
	resp, err := client.Execute(context.Background(), req, resolver, httpclient.Options{BaseDir: baseDir})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

func parseSingleRequest(t *testing.T, path string, src []byte) *restfile.Request {
	t.Helper()
	doc := parser.Parse(path, src)
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d (errors: %+v)", len(doc.Requests), doc.Errors)
	}
	return doc.Requests[0]
}

// Reproduces the reported bug end to end: curl -F import, parse, execute,
// then assert the wire bytes carry curl-equivalent CRLF framing.
func TestExecuteMultipartFromCurlImport(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "testfile.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, rec := newRecordServer(t)
	cmd := `curl -s -F "field1=value1" -F "file=@testfile.txt;type=text/plain" ` + srv.URL + `/upload`
	out := filepath.Join(dir, "curl.http")
	svc := importer.Service{Writer: importer.NewFileWriter()}
	if err := svc.GenerateHTTPFile(context.Background(), cmd, out, importer.WriterOptions{}); err != nil {
		t.Fatalf("generate http file: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	req := parseSingleRequest(t, out, raw)
	executeRequest(t, req, vars.NewResolver(), dir)

	b := rec.boundary(t)
	want := "--" + b + "\r\n" +
		"Content-Disposition: form-data; name=\"field1\"\r\n" +
		"\r\n" +
		"value1\r\n" +
		"--" + b + "\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"testfile.txt\"\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"hello\n\r\n" +
		"--" + b + "--\r\n"
	if string(rec.body) != want {
		t.Errorf("unexpected wire body:\nwant %q\n got %q", want, rec.body)
	}

	values, files := rec.form(t)
	if values["field1"] != "value1" {
		t.Errorf("field1 = %q", values["field1"])
	}
	if string(files["file"]) != "hello\n" {
		t.Errorf("file = %q", files["file"])
	}
}

// Hand-written multipart bodies must produce identical wire framing whether the
// .http file was authored with LF or CRLF line endings.
func TestExecuteMultipartAuthoredLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name string
		eol  string
	}{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			srv, rec := newRecordServer(t)
			src := strings.Join([]string{
				"POST " + srv.URL + "/upload",
				"Content-Type: multipart/form-data; boundary=xxBOUNDxx",
				"",
				"--xxBOUNDxx",
				`Content-Disposition: form-data; name="a"`,
				"",
				"va",
				"--xxBOUNDxx",
				`Content-Disposition: form-data; name="note"`,
				"",
				"# comment-like part content",
				"--xxBOUNDxx--",
				"",
			}, tc.eol)

			req := parseSingleRequest(t, "upload.http", []byte(src))
			executeRequest(t, req, vars.NewResolver(), "")

			values, _ := rec.form(t)
			if values["a"] != "va" {
				t.Errorf("a = %q, wire: %q", values["a"], rec.body)
			}
			if values["note"] != "# comment-like part content" {
				t.Errorf("note = %q, wire: %q", values["note"], rec.body)
			}
		})
	}
}

// Included file bytes must pass through verbatim; CRLF framing applies only to
// the surrounding multipart structure.
func TestExecuteMultipartIncludeKeepsFileBytes(t *testing.T) {
	dir := t.TempDir()
	blob := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x00, '\n', '\r', 0xff, '\n'}
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), blob, 0o644); err != nil {
		t.Fatal(err)
	}

	srv, rec := newRecordServer(t)
	src := "POST " + srv.URL + "/upload\n" +
		"Content-Type: multipart/form-data; boundary=xxBOUNDxx\n" +
		"\n" +
		"--xxBOUNDxx\n" +
		"Content-Disposition: form-data; name=\"f\"; filename=\"blob.bin\"\n" +
		"Content-Type: application/octet-stream\n" +
		"\n" +
		"@blob.bin\n" +
		"--xxBOUNDxx--\n"

	req := parseSingleRequest(t, "blob.http", []byte(src))
	executeRequest(t, req, vars.NewResolver(), dir)

	_, files := rec.form(t)
	if !bytes.Equal(files["f"], blob) {
		t.Errorf("file bytes corrupted:\nwant %q\n got %q", blob, files["f"])
	}
}

// CRLF framing applies to every multipart subtype, not only form-data.
func TestExecuteMultipartMixedUsesCRLFFraming(t *testing.T) {
	srv, rec := newRecordServer(t)
	src := "POST " + srv.URL + "/batch\n" +
		"Content-Type: multipart/mixed; boundary=xxBOUNDxx\n" +
		"\n" +
		"--xxBOUNDxx\n" +
		"Content-Type: application/json\n" +
		"\n" +
		"{\"id\": 1}\n" +
		"--xxBOUNDxx--\n"

	req := parseSingleRequest(t, "batch.http", []byte(src))
	executeRequest(t, req, vars.NewResolver(), "")

	mr := multipart.NewReader(bytes.NewReader(rec.body), "xxBOUNDxx")
	part, err := mr.NextPart()
	if err != nil {
		t.Fatalf("parse multipart body: %v\nwire: %q", err, rec.body)
	}
	if got := part.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("part content type = %q", got)
	}
	data, err := io.ReadAll(part)
	if err != nil {
		t.Fatalf("read part: %v", err)
	}
	if string(data) != `{"id": 1}` {
		t.Errorf("part body = %q", data)
	}
	if _, err := mr.NextPart(); err != io.EOF {
		t.Errorf("expected single part, got err %v", err)
	}
}

// Template expansion runs before CRLF framing and include injection.
func TestExecuteMultipartExpandsTemplates(t *testing.T) {
	srv, rec := newRecordServer(t)
	src := "POST " + srv.URL + "/upload\n" +
		"Content-Type: multipart/form-data; boundary=xxBOUNDxx\n" +
		"\n" +
		"--xxBOUNDxx\n" +
		"Content-Disposition: form-data; name=\"greet\"\n" +
		"\n" +
		"hello {{who}}\n" +
		"--xxBOUNDxx--\n"

	req := parseSingleRequest(t, "tpl.http", []byte(src))
	resolver := vars.NewResolver(vars.NewMapProvider("test", map[string]string{"who": "world"}))
	executeRequest(t, req, resolver, "")

	values, _ := rec.form(t)
	if values["greet"] != "hello world" {
		t.Errorf("greet = %q", values["greet"])
	}
}
