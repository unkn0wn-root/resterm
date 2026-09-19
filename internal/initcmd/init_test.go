package initcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunStandardCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "standard", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{
		fileRequests,
		fileEnv,
		fileEnvExample,
		fileHelp,
		fileRTSHelpers,
		gitignoreFile,
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "resterm.env.json") {
		t.Fatalf("expected resterm.env.json in .gitignore")
	}
}

func TestBuiltinRequestTemplatesUseLocalRunnableMocks(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		wantRequests int
	}{
		{name: "minimal", source: reqHTTPMinimal, wantRequests: 2},
		{name: "standard", source: reqHTTPStandard, wantRequests: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.Contains(tt.source, "httpbin.org") {
				t.Fatal("starter request template must not depend on httpbin.org")
			}
			doc := parser.Parse("requests.http", []byte(tt.source))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %+v", doc.Errors)
			}
			if got := len(doc.Mocks); got != 3 {
				t.Fatalf("mocks = %d, want 3", got)
			}
			if got := len(doc.Requests); got != tt.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tt.wantRequests)
			}
			if _, err := mock.Compile([]*restfile.Document{doc}); err != nil {
				t.Fatalf("compile mocks: %v", err)
			}
		})
	}

	for name, source := range map[string]string{
		"environment":         envJSON,
		"example environment": envExampleJSON,
	} {
		if strings.Contains(source, "httpbin.org") || strings.Contains(source, "api.example.com") {
			t.Fatalf("%s template must use only the local starter service", name)
		}
		if !strings.Contains(source, `"$shared"`) || strings.Count(source, `"url"`) != 1 {
			t.Fatalf("%s template must define the common URL once under $shared", name)
		}
	}

	for _, want := range []string{
		`# @match json={"kind":"greeting"}`,
		`# @match headers={"Authorization":{"prefix":"Bearer "}} json={"role":"member"}`,
		`# @match json-rules={"age":{"gte":18}}`,
		`# @for-each ["david","damian","bob"] as name`,
		`# @for-each helpers.users() as user`,
		`user["nickname"] ?? user.name`,
		`user.active ? "active" : "inactive"`,
	} {
		if !strings.Contains(reqHTTPStandard, want) && !strings.Contains(helpersRTS, want) {
			t.Fatalf("standard template is missing %q", want)
		}
	}
}

func TestStarterUserMockSelectsRulesAndFallback(t *testing.T) {
	doc := parser.Parse("requests.http", []byte(reqHTTPMinimal))
	handler, err := mock.Compile([]*restfile.Document{doc})
	if err != nil {
		t.Fatalf("compile mocks: %v", err)
	}

	request := func(age int, authorized bool) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(
			`{"id":"user-1","name":"Ada","role":"member","age":%d,"displayName":"Ada","status":"active"}`,
			age,
		)
		req := httptest.NewRequest(http.MethodPost, "http://starter.test/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authorized {
			req.Header.Set("Authorization", "Bearer dev-token-123")
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	accepted := request(36, true)
	if accepted.Code != http.StatusCreated || !strings.Contains(accepted.Body.String(), `"name": "Ada"`) {
		t.Fatalf("accepted response = %d %q", accepted.Code, accepted.Body.String())
	}

	for _, res := range []*httptest.ResponseRecorder{request(17, true), request(36, false)} {
		if res.Code != http.StatusUnprocessableEntity {
			t.Fatalf("fallback response = %d %q, want 422", res.Code, res.Body.String())
		}
	}
}

func TestRunConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestRunConflictDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestRunForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Force: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimSpace(string(data)) == "old" {
		t.Fatalf("expected overwrite")
	}
}

func TestRunDry(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "minimal", DryRun: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests.http")); !os.IsNotExist(err) {
		t.Fatalf("expected no files in dry-run")
	}
}

func TestRunNoGitignore(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "minimal", NoGitignore: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, gitignoreFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no .gitignore when no-gitignore is set")
	}
}

func TestListTemplates(t *testing.T) {
	var buf bytes.Buffer
	if err := Run(Opt{List: true, Out: &buf}); err != nil {
		t.Fatalf("list: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "minimal") || !strings.Contains(out, "standard") {
		t.Fatalf("expected template names in output: %s", out)
	}
}

func TestHasGitignoreEntryWithComment(t *testing.T) {
	data := "resterm.env.json # local\n"
	if !hasGitignoreEntry(data, "resterm.env.json") {
		t.Fatalf("expected entry to match with trailing comment")
	}
}

func TestHasGitignoreEntryWithSlash(t *testing.T) {
	data := "/resterm.env.json\n"
	if !hasGitignoreEntry(data, "resterm.env.json") {
		t.Fatalf("expected entry to match with leading slash")
	}
}

func TestGitignoreAppendAddsNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != "node_modules\nresterm.env.json\n" {
		t.Fatalf("unexpected .gitignore content: %q", string(data))
	}
}

func TestGitignoreAppendPreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules\n"), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat .gitignore: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), 0o600)
	}
}

var errBoom = errors.New("boom")

// flakyFS fails the listed CreateTemp or Rename calls, counted from one.
type flakyFS struct {
	FS
	failTemp   int
	failRename []int
	temps      int
	renames    int
}

func (f *flakyFS) CreateTemp(d, pat string) (TempFile, error) {
	f.temps++
	if f.temps == f.failTemp {
		return nil, errBoom
	}
	return f.FS.CreateTemp(d, pat)
}

func (f *flakyFS) Rename(a, b string) error {
	f.renames++
	if slices.Contains(f.failRename, f.renames) {
		return errBoom
	}
	return f.FS.Rename(a, b)
}

func runWith(t *testing.T, fsys FS, o Opt) error {
	t.Helper()
	c := &Command{fs: fsys, templates: BuiltinTemplates{}, out: io.Discard}
	o.Out = io.Discard
	return c.Run(o)
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestRunStageFailureKeepsOldFiles(t *testing.T) {
	dir := t.TempDir()
	req := filepath.Join(dir, "requests.http")
	env := filepath.Join(dir, "resterm.env.json")
	for _, p := range []string{req, env} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	fsys := &flakyFS{FS: OSFS{}, failTemp: 2}
	err := runWith(t, fsys, Opt{Dir: dir, Template: "minimal", Force: true})
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "resterm.env.json") {
		t.Fatalf("err = %v, want write error for resterm.env.json", err)
	}
	if got := readFile(t, req); got != "old" {
		t.Fatalf("requests.http = %q, want old content", got)
	}
	if got := readFile(t, env); got != "old" {
		t.Fatalf("resterm.env.json = %q, want old content", got)
	}
	if names := dirNames(t, dir); !slices.Equal(names, []string{"requests.http", "resterm.env.json"}) {
		t.Fatalf("dir = %v, want only the original files", names)
	}
}

func TestRunCommitFailureRemovesCreated(t *testing.T) {
	dir := t.TempDir()
	fsys := &flakyFS{FS: OSFS{}, failRename: []int{2}}
	err := runWith(t, fsys, Opt{Dir: dir, Template: "minimal"})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want rename error", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("dir = %v, want empty", names)
	}
}

func TestRunCommitFailureRestoresOld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on windows")
	}
	dir := t.TempDir()
	req := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(req, []byte("old"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsys := &flakyFS{FS: OSFS{}, failRename: []int{2}}
	err := runWith(t, fsys, Opt{Dir: dir, Template: "minimal", Force: true})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want rename error", err)
	}
	if got := readFile(t, req); got != "old" {
		t.Fatalf("requests.http = %q, want old content", got)
	}
	info, err := os.Stat(req)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), 0o600)
	}
	if names := dirNames(t, dir); !slices.Equal(names, []string{"requests.http"}) {
		t.Fatalf("dir = %v, want only requests.http", names)
	}
}

func TestRunGitignoreFailureWritesNothing(t *testing.T) {
	dir := t.TempDir()
	fsys := &flakyFS{FS: OSFS{}, failTemp: 3}
	err := runWith(t, fsys, Opt{Dir: dir, Template: "minimal"})
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), gitignoreFile) {
		t.Fatalf("err = %v, want write error for .gitignore", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Fatalf("dir = %v, want empty", names)
	}
}

func TestRunRollbackFailureLeavesNoTemps(t *testing.T) {
	dir := t.TempDir()
	req := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(req, []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The second rename fails the commit and the third fails the restore.
	fsys := &flakyFS{FS: OSFS{}, failRename: []int{2, 3}}
	err := runWith(t, fsys, Opt{Dir: dir, Template: "minimal", Force: true})
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "restore requests.http") {
		t.Fatalf("err = %v, want restore error for requests.http", err)
	}
	if names := dirNames(t, dir); !slices.Equal(names, []string{"requests.http"}) {
		t.Fatalf("dir = %v, want only requests.http", names)
	}
}
