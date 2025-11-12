package vars

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

func TestIsDotEnvPath(t *testing.T) {
	testCases := []struct {
		path string
		want bool
	}{
		{"resterm.env.json", false},
		{"resterm.env", true},
		{".env", true},
		{".env.prod", true},
		{"/tmp/prod.env", true},
		{"/tmp/env", false},
		{"/tmp/RESTERM.ENV.JSON", false},
	}
	for _, tc := range testCases {
		if got := IsDotEnvPath(tc.path); got != tc.want {
			t.Fatalf("IsDotEnvPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestLoadEnvironmentFileDotEnvWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.stage")

	t.Setenv("CLI_REGION", "us-east-1")

	content := `
# comment
workspace=stage
BASE_URL = https://api.example.com
API_URL=${BASE_URL}/v1
TOKEN="${API_URL}/token"
LITERAL='${API_URL}'
TIMEOUT=30 # seconds
REGION = ${CLI_REGION}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	envs, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}
	env := envs["stage"]
	if env == nil {
		t.Fatalf("expected stage environment to exist: %#v", envs)
	}
	if env["BASE_URL"] != "https://api.example.com" {
		t.Fatalf("BASE_URL = %q, want %q", env["BASE_URL"], "https://api.example.com")
	}
	if env["API_URL"] != "https://api.example.com/v1" {
		t.Fatalf("API_URL = %q, want %q", env["API_URL"], "https://api.example.com/v1")
	}
	if env["TOKEN"] != "https://api.example.com/v1/token" {
		t.Fatalf("TOKEN = %q, want %q", env["TOKEN"], "https://api.example.com/v1/token")
	}
	if env["LITERAL"] != "${API_URL}" {
		t.Fatalf("LITERAL = %q, want literal ${API_URL}", env["LITERAL"])
	}
	if env["TIMEOUT"] != "30" {
		t.Fatalf("TIMEOUT = %q, want %q", env["TIMEOUT"], "30")
	}
	if env["REGION"] != "us-east-1" {
		t.Fatalf("REGION = %q, want %q", env["REGION"], "us-east-1")
	}
	if env["workspace"] != "stage" {
		t.Fatalf("workspace = %q, want %q", env["workspace"], "stage")
	}
}

func TestLoadEnvironmentFileDotEnvNameFallbacks(t *testing.T) {
	dir := t.TempDir()

	prodPath := filepath.Join(dir, "prod.env")
	if err := os.WriteFile(prodPath, []byte("API=https://api\n"), 0o644); err != nil {
		t.Fatalf("write prod.env: %v", err)
	}
	envs, err := LoadEnvironmentFile(prodPath)
	if err != nil {
		t.Fatalf("load prod env: %v", err)
	}
	if _, ok := envs["prod"]; !ok {
		t.Fatalf("expected environment derived from filename 'prod', got keys %v", mapsKeys(envs))
	}

	defaultPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(defaultPath, []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	envs, err = LoadEnvironmentFile(defaultPath)
	if err != nil {
		t.Fatalf("load default env: %v", err)
	}
	if _, ok := envs["default"]; !ok {
		t.Fatalf("expected default environment, got keys %v", mapsKeys(envs))
	}
}

func TestLoadEnvironmentFileDotEnvMissingReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.env")
	if err := os.WriteFile(path, []byte("URL=${MISSING}\n"), 0o644); err != nil {
		t.Fatalf("write broken env: %v", err)
	}

	_, err := LoadEnvironmentFile(path)
	if err == nil {
		t.Fatalf("expected error for missing reference")
	}
	if errdef.CodeOf(err) != errdef.CodeParse {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLoadEnvironmentFileDotEnvDuplicateWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.env")
	content := "workspace=dev\nWORKSPACE=prod\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write dup env: %v", err)
	}

	_, err := LoadEnvironmentFile(path)
	if err == nil {
		t.Fatalf("expected duplicate workspace error")
	}
	if errdef.CodeOf(err) != errdef.CodeParse {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func mapsKeys(set EnvironmentSet) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	return keys
}
