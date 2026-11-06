package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"go.yaml.in/yaml/v4"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

const (
	maxNameLen        = 64
	maxDescriptionLen = 1024
	maxSkillLines     = 500
)

var (
	skillName     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	httpBlock     = regexp.MustCompile("(?s)```http\n(.*?)```")
	jsonBlock     = regexp.MustCompile("(?s)```json\n(.*?)```")
	directiveSpan = regexp.MustCompile("`(?:# )?@([a-z][a-z0-9-]*)(?: [^`\n]*)?`")
	mdLink        = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	exitRow       = regexp.MustCompile("(?m)^\\| `(\\d+)` \\|")
)

var exitCodes = []int{
	runfail.ExitPass,
	runfail.ExitFailure,
	runfail.ExitUsage,
	runfail.ExitInternal,
	runfail.ExitTimeout,
	runfail.ExitNetwork,
	runfail.ExitTLS,
	runfail.ExitAuth,
	runfail.ExitScript,
	runfail.ExitFilesystem,
	runfail.ExitProtocol,
	runfail.ExitRoute,
	runfail.ExitCanceled,
}

func TestSkillFrontmatter(t *testing.T) {
	data, err := fs.ReadFile(skillFS(t), "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFrontmatter(Name, data); err != nil {
		t.Fatalf("SKILL.md: %v", err)
	}

	head := func(name, desc string) string {
		return "---\nname: " + name + "\ndescription: " + desc + "\n---\n"
	}
	tests := []struct {
		name string
		dir  string
		data string
		ok   bool
	}{
		{name: "hyphenated name", dir: "api-tools-2", data: head("api-tools-2", "Run requests."), ok: true},
		{name: "longest description", dir: "x", data: head("x", strings.Repeat("é", maxDescriptionLen)), ok: true},
		{name: "longest file", dir: "x", data: head("x", "d") + strings.Repeat("\n", maxSkillLines-5), ok: true},
		{name: "no frontmatter", dir: "x", data: "# x\n"},
		{name: "unclosed frontmatter", dir: "x", data: "---\nname: x\ndescription: d\n"},
		{name: "upper case name", dir: "Resterm", data: head("Resterm", "d")},
		{name: "double hyphen", dir: "a--b", data: head("a--b", "d")},
		{name: "long name", dir: strings.Repeat("a", maxNameLen+1), data: head(strings.Repeat("a", maxNameLen+1), "d")},
		{name: "folder mismatch", dir: "resterm", data: head("using-resterm", "d")},
		{name: "extra key", dir: "x", data: "---\nname: x\ndescription: d\nversion: 1\n---\n"},
		{name: "angle brackets", dir: "x", data: head("x", "Install it in a dir/x folder, not <dir>.")},
		{name: "empty description", dir: "x", data: head("x", `""`)},
		{name: "long description", dir: "x", data: head("x", strings.Repeat("a", maxDescriptionLen+1))},
		{name: "too many lines", dir: "x", data: head("x", "d") + strings.Repeat("\n", maxSkillLines-4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkFrontmatter(tt.dir, []byte(tt.data))
			if (err == nil) != tt.ok {
				t.Fatalf("checkFrontmatter(%q) = %v, want ok %t", tt.dir, err, tt.ok)
			}
		})
	}
}

func TestSkillExamplesParse(t *testing.T) {
	if err := checkExamples(skillFS(t)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		http string
		ok   bool
	}{
		{name: "capture", http: "### A\n# @capture file x = 1\nGET https://example.com\n", ok: true},
		{name: "unknown directive", http: "### A\n# @captur file x = 1\nGET https://example.com\n"},
		{name: "unknown capture scope", http: "### A\n# @capture everywhere x = 1\nGET https://example.com\n"},
		{
			name: "workflow typo",
			http: "### W\n# @workflow w\n# @step S using=A\n# @stpe T using=A\n\n### A\n# @name A\nGET https://example.com\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{"SKILL.md": {Data: []byte("```http\n" + tt.http + "```\n")}}
			if err := checkExamples(fsys); (err == nil) != tt.ok {
				t.Fatalf("checkExamples = %v, want ok %t", err, tt.ok)
			}
		})
	}
}

func TestSkillDirectivesKnown(t *testing.T) {
	if err := checkDirectives(skillFS(t)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{name: "known", text: "Use `@for-each`, `# @capture file x = 1` and `@grpc*` lines.", ok: true},
		{name: "unknown", text: "Use `@for-eech` to repeat a request."},
		{name: "unknown with comment marker", text: "Write `# @captur file x = 1`."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{"SKILL.md": {Data: []byte(tt.text)}}
			if err := checkDirectives(fsys); (err == nil) != tt.ok {
				t.Fatalf("checkDirectives(%q) = %v, want ok %t", tt.text, err, tt.ok)
			}
		})
	}
}

func TestSkillReferencesLinked(t *testing.T) {
	if err := checkLinks(skillFS(t)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		fsys fstest.MapFS
		ok   bool
	}{
		{
			name: "linked",
			fsys: fstest.MapFS{
				"SKILL.md": {
					Data: []byte(
						"[run](references/run.md#flags), [requests](references/requests.md), [docs](https://resterm.app/)",
					),
				},
				"references/run.md":      {Data: []byte("Back to [requests](requests.md).")},
				"references/requests.md": {Data: []byte("x")},
			},
			ok: true,
		},
		{
			name: "missing target",
			fsys: fstest.MapFS{"SKILL.md": {Data: []byte("See [run](references/missing.md).")}},
		},
		{
			name: "unlinked file",
			fsys: fstest.MapFS{
				"SKILL.md":            {Data: []byte("See [run](references/run.md).")},
				"references/run.md":   {Data: []byte("x")},
				"references/extra.md": {Data: []byte("x")},
			},
		},
		{
			name: "linked only from a reference",
			fsys: fstest.MapFS{
				"SKILL.md":           {Data: []byte("See [run](references/run.md).")},
				"references/run.md":  {Data: []byte("See [deep](deep.md).")},
				"references/deep.md": {Data: []byte("x")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkLinks(tt.fsys); (err == nil) != tt.ok {
				t.Fatalf("checkLinks = %v, want ok %t", err, tt.ok)
			}
		})
	}
}

func TestSkillExitCodes(t *testing.T) {
	skill, err := fs.ReadFile(skillFS(t), "SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := os.ReadFile("../docs/cli/run.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkExitCodes(skill); err != nil {
		t.Errorf("SKILL.md: %v", err)
	}
	if err := checkExitCodes(docs); err != nil {
		t.Errorf("docs/cli/run.md: %v", err)
	}

	var table strings.Builder
	for _, code := range exitCodes {
		fmt.Fprintf(&table, "| `%d` | meaning |\n", code)
	}
	full := table.String()
	tests := []struct {
		name  string
		table string
		ok    bool
	}{
		{name: "every code", table: full, ok: true},
		{name: "missing 27", table: strings.Replace(full, "| `27` | meaning |\n", "", 1)},
		{name: "extra 28", table: strings.Replace(full, "| `130` |", "| `28` | meaning |\n| `130` |", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkExitCodes([]byte(tt.table)); (err == nil) != tt.ok {
				t.Fatalf("checkExitCodes = %v, want ok %t", err, tt.ok)
			}
		})
	}
}

func TestSkillReportExample(t *testing.T) {
	data, err := fs.ReadFile(skillFS(t), "references/run.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkReport(data); err != nil {
		t.Fatalf("references/run.md: %v", err)
	}

	example := jsonBlock.FindSubmatch(data)
	if example == nil {
		t.Fatal("references/run.md has no json example")
	}
	tests := []struct {
		name string
		data string
		ok   bool
	}{
		{name: "example", data: string(example[0]), ok: true},
		{name: "renamed key", data: strings.Replace(string(example[0]), `"bodySize"`, `"size"`, 1)},
		{name: "no example", data: "No report here.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkReport([]byte(tt.data)); (err == nil) != tt.ok {
				t.Fatalf("checkReport = %v, want ok %t", err, tt.ok)
			}
		})
	}
}

func skillFS(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(FS, Name)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func walkFiles(fsys fs.FS, fn func(name string, data []byte)) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		fn(name, data)
		return nil
	})
}

func checkFrontmatter(dir string, data []byte) error {
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return errors.New("no frontmatter at the start of the file")
	}
	head, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return errors.New("frontmatter is not closed")
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Load(head, &meta, yaml.WithKnownFields()); err != nil {
		return err
	}
	n := utf8.RuneCountInString(meta.Description)
	lines := bytes.Count(data, []byte("\n"))
	switch {
	case len(meta.Name) > maxNameLen || !skillName.MatchString(meta.Name):
		return fmt.Errorf("name %q is not up to %d lowercase letters, digits and single hyphens", meta.Name, maxNameLen)
	case meta.Name != dir:
		return fmt.Errorf("name %q does not match the folder %q", meta.Name, dir)
	case n == 0 || n > maxDescriptionLen:
		return fmt.Errorf("description has %d characters, want 1 to %d", n, maxDescriptionLen)
	case strings.ContainsAny(meta.Description, "<>"):
		return errors.New("description contains < or >")
	case lines >= maxSkillLines:
		return fmt.Errorf("file has %d lines, want fewer than %d", lines, maxSkillLines)
	}
	return nil
}

func checkExamples(fsys fs.FS) error {
	var errs []error
	err := walkFiles(fsys, func(name string, data []byte) {
		for _, m := range httpBlock.FindAllSubmatch(data, -1) {
			doc := parser.Parse(name, m[1])
			for _, d := range slices.Concat(doc.Errors, doc.Warnings) {
				errs = append(errs, fmt.Errorf("%s: %s in\n%s", name, d.Message, m[1]))
			}
		}
	})
	return errors.Join(append(errs, err)...)
}

func checkDirectives(fsys fs.FS) error {
	var errs []error
	err := walkFiles(fsys, func(name string, data []byte) {
		for _, m := range directiveSpan.FindAllSubmatch(data, -1) {
			if !directive.Name(m[1]).Known() {
				errs = append(errs, fmt.Errorf("%s: @%s is not a known directive", name, m[1]))
			}
		}
	})
	return errors.Join(append(errs, err)...)
}

func checkLinks(fsys fs.FS) error {
	var errs []error
	var names []string
	linked := map[string]bool{}
	err := walkFiles(fsys, func(name string, data []byte) {
		names = append(names, name)
		for _, m := range mdLink.FindAllSubmatch(data, -1) {
			target, _, _ := strings.Cut(string(m[1]), "#")
			if target == "" || strings.Contains(target, "://") {
				continue
			}
			p := path.Join(path.Dir(name), target)
			if _, err := fs.Stat(fsys, p); err != nil {
				errs = append(errs, fmt.Errorf("%s links to %s, which does not exist", name, target))
			}
			if name == "SKILL.md" {
				linked[p] = true
			}
		}
	})
	for _, name := range names {
		if name != "SKILL.md" && !linked[name] {
			errs = append(errs, fmt.Errorf("SKILL.md does not link to %s", name))
		}
	}
	return errors.Join(append(errs, err)...)
}

func checkExitCodes(data []byte) error {
	var got []int
	for _, m := range exitRow.FindAllSubmatch(data, -1) {
		code, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return err
		}
		got = append(got, code)
	}
	if !slices.Equal(got, exitCodes) {
		return fmt.Errorf("exit code table lists %v, want %v", got, exitCodes)
	}
	return nil
}

func checkReport(data []byte) error {
	blocks := jsonBlock.FindAllSubmatch(data, -1)
	if len(blocks) == 0 {
		return errors.New("no json report example")
	}
	for _, m := range blocks {
		var rep runfmt.Report
		if err := json.Unmarshal(m[1], &rep); err != nil {
			return err
		}
		out, err := json.Marshal(rep)
		if err != nil {
			return err
		}
		var want, got any
		if err := json.Unmarshal(m[1], &want); err != nil {
			return err
		}
		if err := json.Unmarshal(out, &got); err != nil {
			return err
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("report example changes in a round trip through the report type:\n%s", out)
		}
	}
	return nil
}
