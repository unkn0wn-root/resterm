package main

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/skills"
)

var (
	skillCommandLine = regexp.MustCompile("\\bresterm ([a-z]+)([^`\n|]*)")
	skillTableRow    = regexp.MustCompile(`(?m)^\|.*`)
	skillCodeSpan    = regexp.MustCompile("`([^`\n]*)`")
)

func TestSkillPrints(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "skills", "resterm", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	out, errOut, err := captureRunIO(t, func() error { return run([]string{"skill"}) })
	if err != nil {
		t.Fatalf("run(skill): %v", err)
	}
	if out != string(want) {
		t.Fatalf("run(skill) printed %d bytes, want SKILL.md unchanged (%d bytes)", len(out), len(want))
	}
	if errOut != "" {
		t.Fatalf("run(skill) stderr = %q, want empty", errOut)
	}
}

func TestSkillUsage(t *testing.T) {
	const usage = `Usage: resterm skill [--agent <name>... | --dir <dir>]

Print the resterm skill for coding agents.
With --agent, install it where that agent reads skills. With --dir, install it in <dir>/resterm.

Flags:
  --agent string  Agent to install for: claude, codex (repeatable)
  --dir string    Skills directory to install the skill into
`

	tests := []struct {
		args []string
		out  string
		code int
		err  string
	}{
		{args: []string{"--help"}, out: usage},
		{args: []string{"--bogus"}, code: cli.ExitUsage},
		{args: []string{"extra"}, code: cli.ExitUsage},
		{args: []string{"--dir", ""}, code: cli.ExitUsage},
		{
			args: []string{"--agent", "cursor"},
			code: cli.ExitUsage,
			err:  `unknown agent "cursor". Use claude, codex, or --dir`,
		},
		{args: []string{"--agent", "claude,codex"}, code: cli.ExitUsage},
		{args: []string{"--agent", ""}, code: cli.ExitUsage},
		{args: []string{"--agent", "claude", "--agent", "cursor"}, code: cli.ExitUsage},
		{args: []string{"--agent", "claude", "--dir", "d"}, code: cli.ExitUsage, err: "use --agent or --dir, not both"},
		{args: []string{"--agent", "codex"}, code: runfail.ExitFailure, err: "is not defined"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			t.Setenv("HOME", "")
			t.Setenv("USERPROFILE", "")
			t.Setenv("CLAUDE_CONFIG_DIR", "")
			out, _, err := captureRunIO(t, func() error { return runSkill(tt.args) })
			if cli.ExitCode(err) != tt.code || (err != nil && !strings.Contains(err.Error(), tt.err)) {
				t.Fatalf("runSkill(%q) = %v, want exit %d with %q", tt.args, err, tt.code, tt.err)
			}
			if out != tt.out {
				t.Fatalf("runSkill(%q) stdout = %q, want %q", tt.args, out, tt.out)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("runSkill(%q) wrote files", tt.args)
			}
		})
	}
}

func TestSkillInstall(t *testing.T) {
	const (
		claudeSkills = ".claude/skills/resterm"
		codexSkills  = ".agents/skills/resterm"
	)
	sub, err := fs.Sub(skills.FS, skills.Name)
	if err != nil {
		t.Fatal(err)
	}
	skill := readTree(t, sub)

	tests := []struct {
		name      string
		args      []string
		claudeDir string
		blocked   []string
		want      []string
	}{
		{name: "dir", args: []string{"--dir", "$HOME/a/b"}, want: []string{"a/b/resterm"}},
		{
			name: "codex and claude",
			args: []string{"--agent", "codex", "--agent", "claude"},
			want: []string{codexSkills, claudeSkills},
		},
		{
			name:      "CLAUDE_CONFIG_DIR",
			args:      []string{"--agent", "claude"},
			claudeDir: "$HOME/work",
			want:      []string{"work/skills/resterm"},
		},
		{
			name:    "codex after claude fails",
			args:    []string{"--agent", "claude", "--agent", "codex"},
			blocked: []string{claudeSkills},
			want:    []string{codexSkills},
		},
		{
			name:    "both fail",
			args:    []string{"--agent", "claude", "--agent", "codex"},
			blocked: []string{claudeSkills, codexSkills},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Chdir(home)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
			t.Setenv("CLAUDE_CONFIG_DIR", os.ExpandEnv(tt.claudeDir))
			args := make([]string, len(tt.args))
			for i, arg := range tt.args {
				args[i] = os.ExpandEnv(arg)
			}

			files := map[string]string{}
			for _, dst := range tt.blocked {
				p := filepath.Join(home, dst)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				files[dst] = ""
			}
			code := 0
			if len(tt.blocked) > 0 {
				code = runfail.ExitFailure
			}
			var want string
			for _, dst := range tt.want {
				want += "Installed the resterm skill in " + filepath.Join(home, dst) + "\n"
				for name, data := range skill {
					files[path.Join(dst, name)] = data
				}
			}

			install := func() {
				t.Helper()
				out, errOut, err := captureRunIO(t, func() error { return runSkill(args) })
				if cli.ExitCode(err) != code {
					t.Fatalf("runSkill(%q) = %v, want exit %d", args, err, code)
				}
				for _, dst := range tt.blocked {
					if p := filepath.Join(home, dst); !strings.Contains(err.Error(), p) {
						t.Fatalf("runSkill(%q) = %v, want an error for %s", args, err, p)
					}
				}
				if len(tt.blocked) > 0 {
					head, _, _ := strings.Cut(diag.Render(err), "\n")
					if line := "error[unknown]: " + strings.ReplaceAll(err.Error(), "\n", "; "); head != line {
						t.Fatalf("header = %q, want %q", head, line)
					}
				}
				if out != want {
					t.Fatalf("stdout = %q, want %q", out, want)
				}
				if errOut != "" {
					t.Fatalf("stderr = %q, want empty", errOut)
				}
				for _, dst := range tt.want {
					p := filepath.Join(home, dst)
					if entries, err := os.ReadDir(filepath.Dir(p)); err != nil || len(entries) != 1 {
						t.Fatalf("ReadDir(%s) = %v, %v, want only %s", filepath.Dir(p), entries, err, skills.Name)
					}
					top, err := os.Stat(p)
					if err != nil {
						t.Fatal(err)
					}
					refs, err := os.Stat(filepath.Join(p, "references"))
					if err != nil {
						t.Fatal(err)
					}
					if top.Mode() != refs.Mode() {
						t.Fatalf("%s mode = %v, want %v like its references folder", p, top.Mode(), refs.Mode())
					}
				}
			}
			install()
			for _, dst := range tt.want {
				stale := filepath.Join(home, dst, "references", "old.md")
				if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			install()

			if got := readTree(t, os.DirFS(home)); !maps.Equal(got, files) {
				t.Fatalf("files %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(files)))
			}
		})
	}
}

func TestSkillInstallRefusesOtherFiles(t *testing.T) {
	skill, err := skills.FS.ReadFile(path.Join(skills.Name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		files map[string]string
		links map[string]string
		want  string
	}{
		{
			name:  "directory without SKILL.md",
			files: map[string]string{"resterm/go.mod": "module example.com/resterm\n"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "regular file",
			files: map[string]string{"resterm": "\x7fELF binary"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "SKILL.md folder",
			files: map[string]string{"resterm/SKILL.md/notes.md": "notes\n", "resterm/important.txt": "keep\n"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "other skill",
			files: map[string]string{"resterm/SKILL.md": "---\nname: other\n---\n"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "frontmatter not at the top",
			files: map[string]string{"resterm/SKILL.md": "# Notes\nname: resterm\n---\nname: resterm\n---\n"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "name after frontmatter",
			files: map[string]string{"resterm/SKILL.md": "---\ndescription: Notes.\n---\nname: resterm\n"},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "SKILL.md link",
			files: map[string]string{"shared/SKILL.md": string(skill), "resterm/important.txt": "keep\n"},
			links: map[string]string{"resterm/SKILL.md": filepath.Join("..", "shared", "SKILL.md")},
			want:  "exists and is not a resterm skill",
		},
		{
			name:  "link to a skill",
			files: map[string]string{"shared/SKILL.md": "old\n", "shared/references/old.md": "old\n"},
			links: map[string]string{"resterm": "shared"},
			want:  "is a link",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tt.files {
				p := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dst := filepath.Join(dir, skills.Name)
			for name, target := range tt.links {
				if err := os.Symlink(target, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
					t.Skipf("cannot create a link: %v", err)
				}
			}
			out, _, err := captureRunIO(t, func() error { return runSkill([]string{"--dir", dir}) })
			if cli.ExitCode(err) != runfail.ExitFailure || !strings.Contains(err.Error(), dst+" "+tt.want) {
				t.Fatalf("runSkill(--dir %s) = %v, want exit 1 with %q", dir, err, dst+" "+tt.want)
			}
			if out != "" {
				t.Fatalf("stdout = %q, want empty", out)
			}
			if got := readTree(t, os.DirFS(dir)); !maps.Equal(got, tt.files) {
				t.Fatalf("files changed to %v, want %v", got, tt.files)
			}
			for name, target := range tt.links {
				p := filepath.Join(dir, filepath.FromSlash(name))
				if got, err := os.Readlink(p); err != nil || got != target {
					t.Fatalf("Readlink(%s) = %q, %v, want %q", p, got, err, target)
				}
			}
		})
	}
}

func TestSkillInstallReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("read-only folders do not stop root or Windows")
	}
	tests := []struct {
		name  string
		skill bool
		lock  string
	}{
		{name: "parent with a skill", skill: true, lock: "."},
		{name: "empty parent", lock: "."},
		{name: "skill folder", skill: true, lock: skills.Name},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			args := []string{"--dir", dir}
			if tt.skill {
				if _, _, err := captureRunIO(t, func() error { return runSkill(args) }); err != nil {
					t.Fatal(err)
				}
			}
			p := filepath.Join(dir, tt.lock)
			if err := os.Chmod(p, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(p, 0o755) })
			want := readTree(t, os.DirFS(dir))
			out, _, err := captureRunIO(t, func() error { return runSkill(args) })
			if cli.ExitCode(err) != runfail.ExitFailure || !errors.Is(err, fs.ErrPermission) {
				t.Fatalf("runSkill(%q) = %v, want exit 1 with a permission error", args, err)
			}
			if out != "" {
				t.Fatalf("stdout = %q, want empty", out)
			}
			if got := readTree(t, os.DirFS(dir)); !maps.Equal(got, want) {
				t.Fatalf("files changed to %v, want %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
			}
		})
	}
}

func TestSkillCommandFlags(t *testing.T) {
	err := fs.WalkDir(skills.FS, skills.Name, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := skills.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := checkCommandFlags(name, data); err != nil {
			t.Error(err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{
			name: "known flags",
			text: "`resterm run -r login --format=json -ff api.http`\n`resterm skill --agent claude`\n| `--fail-fast` | `-ff` | Stop. |",
			ok:   true,
		},
		{name: "unknown run flag", text: "`resterm run --responses api.http`"},
		{name: "unknown skill flag", text: "`resterm skill --agents claude`"},
		{name: "unknown flag in a table", text: "| `--fail-fast` | `-se` | Stop. |"},
		{name: "unchecked command", text: "Start mocks with `resterm mock api.http`."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkCommandFlags(tt.name, []byte(tt.text)); (err == nil) != tt.ok {
				t.Fatalf("checkCommandFlags(%q) = %v, want ok %t", tt.text, err, tt.ok)
			}
		})
	}
}

func checkCommandFlags(name string, data []byte) error {
	flags := map[string]*cli.FlagSet{
		"run":   newRunCmd().fs,
		"skill": newSkillFlags(new(string), new([]string)),
	}
	var errs []error
	check := func(cmd, arg string) {
		flag, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && flag != "" && flags[cmd].Lookup(flag) == nil {
			errs = append(errs, fmt.Errorf("%s: resterm %s has no flag %s", name, cmd, arg))
		}
	}
	for _, m := range skillCommandLine.FindAllStringSubmatch(string(data), -1) {
		if flags[m[1]] == nil {
			errs = append(errs, fmt.Errorf("%s: resterm %s is not a checked command", name, m[1]))
			continue
		}
		args, _, _ := strings.Cut(m[2], " #")
		for _, arg := range strings.Fields(args) {
			check(m[1], arg)
		}
	}
	for _, row := range skillTableRow.FindAllString(string(data), -1) {
		for _, m := range skillCodeSpan.FindAllStringSubmatch(row, -1) {
			arg, _, _ := strings.Cut(m[1], " ")
			check("run", arg)
		}
	}
	return errors.Join(errs...)
}

func readTree(t *testing.T, fsys fs.FS) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		files[name] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
