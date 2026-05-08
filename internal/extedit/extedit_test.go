package extedit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestResolveWith(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantArgs []string
		wantErr  error
	}{
		{
			name: "RESTERM_EDITOR takes precedence",
			env: map[string]string{
				EnvEditor:        "vim",
				EnvVisual:        "code --wait",
				EnvRestermEditor: "nvim -p",
			},
			wantArgs: []string{"nvim", "-p"},
		},
		{
			name: "falls back to VISUAL then EDITOR",
			env: map[string]string{
				EnvEditor: "vim",
				EnvVisual: "code --wait",
			},
			wantArgs: []string{"code", "--wait"},
		},
		{
			name:    "no editor configured",
			wantErr: ErrNoEditor,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := ResolveWith(mapEnv(tt.env), okLookPath)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveWith returned error: %v", err)
			}
			if !reflect.DeepEqual(cmd.Args, tt.wantArgs) {
				t.Fatalf("expected args %v, got %v", tt.wantArgs, cmd.Args)
			}
		})
	}
}

func TestResolveWithMissingExecutable(t *testing.T) {
	_, err := ResolveWith(
		mapEnv(map[string]string{EnvEditor: "missing-editor --wait"}),
		func(string) (string, error) { return "", errors.New("missing") },
	)
	if err == nil || !strings.Contains(err.Error(), `editor "missing-editor" not found`) {
		t.Fatalf("expected missing editor error, got %v", err)
	}
}

func TestResolveWithQuotedEmptyEditor(t *testing.T) {
	_, err := ResolveWith(mapEnv(map[string]string{EnvEditor: `"" --wait`}), okLookPath)
	if !errors.Is(err, ErrNoArgs) {
		t.Fatalf("expected ErrNoArgs, got %v", err)
	}
}

func TestCmdExecAppendsPath(t *testing.T) {
	cmd := Cmd{Args: []string{"code", "--wait"}}
	got := cmd.Exec("/tmp/demo.http").Args
	want := []string{"code", "--wait", "/tmp/demo.http"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected args %v, got %v", want, got)
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "plain args",
			in:   "code --wait",
			want: []string{"code", "--wait"},
		},
		{
			name: "quoted executable",
			in:   `"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" --wait`,
			want: []string{"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code", "--wait"},
		},
		{
			name: "single quoted arg",
			in:   `editor --name 'main workspace'`,
			want: []string{"editor", "--name", "main workspace"},
		},
		{
			name: "single quotes keep backslash literal",
			in:   `editor 'a\b'`,
			want: []string{"editor", `a\b`},
		},
		{
			name: "escaped space",
			in:   `vim\ wrapper --flag`,
			want: []string{"vim wrapper", "--flag"},
		},
		{
			name: "keeps windows separators",
			in:   `"C:\Program Files\Editor\editor.exe" --reuse-window`,
			want: []string{`C:\Program Files\Editor\editor.exe`, "--reuse-window"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := split(tt.in)
			if err != nil {
				t.Fatalf("Split returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestSplitUnterminatedQuote(t *testing.T) {
	if _, err := split(`"code --wait`); err == nil {
		t.Fatalf("expected unterminated quote error")
	}
}

func mapEnv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

func okLookPath(path string) (string, error) {
	return path, nil
}
