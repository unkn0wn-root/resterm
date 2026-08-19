package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"github.com/unkn0wn-root/resterm/internal/watcher"
)

const (
	brokenEnv = `{"dev":{"token":"env:"}}`
	loadedEnv = `{"dev":{"token":"dev-token"},"prod":{"token":"prod-token"}}`
)

func brokenEnvModel(t *testing.T, root string) Model {
	t.Helper()
	cat, envFile, err := vars.Discover(root)
	if err == nil {
		t.Fatal("the environment file loaded, so there is nothing to recover from")
	}
	return New(Config{
		Env:           vars.Config{Catalog: cat, File: envFile, FileErr: err},
		WorkspaceRoot: root,
	})
}

func brokenPinnedEnvModel(t *testing.T, root, pinned string) Model {
	t.Helper()
	cat, err := vars.LoadEnvironmentFile(pinned)
	if err == nil {
		t.Fatal("the environment file loaded, so there is nothing to recover from")
	}
	return New(Config{
		Env:           vars.Config{Catalog: cat, File: pinned, FileExplicit: true, FileErr: err},
		WorkspaceRoot: root,
	})
}

func TestBrokenEnvironmentFileStillOpensTheSession(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": brokenEnv,
		"api.http":         "GET http://example.test/{{token}}\n",
	})
	m := brokenEnvModel(t, root)

	if got := m.statusMessage.level; got != statusError {
		t.Fatalf("level = %v, want error", got)
	}
	for _, want := range []string{
		"Environment file failed to load",
		"env: reference with no variable name",
		envFixHint,
	} {
		if !strings.Contains(m.statusMessage.text, want) {
			t.Fatalf("status = %q, want it to contain %q", m.statusMessage.text, want)
		}
	}

	if !m.showStatusModal {
		t.Fatal("the session opened with the failure only in the status bar")
	}
	if m.statusModalMessage != m.statusMessage.text {
		t.Fatalf("modal = %q, want the status text", m.statusModalMessage)
	}
	if got := m.headerEnvVariants()[0]; got != "not loaded" {
		t.Fatalf("header = %q, want it to show there is no environment", got)
	}
}

func TestBrokenEnvironmentFileRefusesToSend(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": brokenEnv,
		"api.http":         "GET http://example.test/\n",
	})
	model := brokenEnvModel(t, root)
	m := &model

	refused, ok := statusMsgFromCmd(runCmd(m.startRun(runSpec{sel: m.ws.sel})))
	if !ok {
		t.Fatal("the run was not refused")
	}
	if refused.level != statusError {
		t.Fatalf("level = %v, want error", refused.level)
	}
	for _, want := range []string{"Environment file failed to load", envFixHint} {
		if !strings.Contains(refused.text, want) {
			t.Fatalf("refusal = %q, want it to contain %q", refused.text, want)
		}
	}

	binding := bindings.Binding{Action: bindings.ActionOpenEnvSelector}
	opened, _ := statusMsgFromCmd(runCmd(m.runShortcutBinding(binding, tea.KeyMsg{})))
	if !strings.Contains(opened.text, "Environment file failed to load") {
		t.Fatalf("Ctrl+E status = %q, want the reason the list is empty", opened.text)
	}
	if m.showEnvSelector {
		t.Fatal("the picker opened on a catalog that never loaded")
	}
}

func TestStartupWarningsDoNotOpenTheStatusModal(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json":   loadedEnv,
		"a/resterm.env.json": loadedEnv,
		"a/req.http":         "GET http://example.test/\n",
	})
	m := modelInWorkspace(t, root, true)

	if m.statusMessage.level != statusWarn {
		t.Fatalf("level = %v, want the nested file warning", m.statusMessage.level)
	}
	if m.showStatusModal {
		t.Fatalf("a warning opened a modal: %q", m.statusModalMessage)
	}
}

func TestSavingTheEnvironmentFileLoadsIt(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": brokenEnv,
		"api.http":         "GET http://example.test/{{token}}\n",
	})
	model := brokenEnvModel(t, root)
	m := &model

	m.openFile(filepath.Join(root, "resterm.env.json"))
	m.editor.SetValue(loadedEnv)
	status, ok := statusMsgFromCmd(m.saveFile())
	if !ok {
		t.Fatal("save produced no status message")
	}

	if want := "Saved resterm.env.json. Environment dev"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
	if status.level != statusSuccess {
		t.Fatalf("level = %v, want success", status.level)
	}
	if got := m.ws.active.Values()["token"]; got != "dev-token" {
		t.Fatalf("token = %q, want the saved file to be active", got)
	}
	if got := len(m.envList.Items()); got != 2 {
		t.Fatalf("picker items = %d, want both environments", got)
	}
	if cmd := m.runBlocked(); cmd != nil {
		t.Fatal("the session still refuses to send after the file loaded")
	}
}

func TestReloadingTheEnvironmentFileFromDiskLoadsIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reload func(m *Model, path string) tea.Cmd
	}{
		{
			name: "an edit made outside Resterm",
			reload: func(m *Model, path string) tea.Cmd {
				return m.handleFileChangeEvent(
					fileChangedMsg{path: path, kind: watcher.EventChanged},
				)
			},
		},
		{
			name:   "a reload from disk",
			reload: func(m *Model, _ string) tea.Cmd { return m.reloadFileFromDisk() },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeWorkspace(t, map[string]string{
				"resterm.env.json": brokenEnv,
				"api.http":         "GET http://example.test/{{token}}\n",
			})
			model := brokenEnvModel(t, root)
			m := &model

			path := filepath.Join(root, "resterm.env.json")
			m.openFile(path)
			if err := os.WriteFile(path, []byte(loadedEnv), 0o644); err != nil {
				t.Fatal(err)
			}

			status, ok := statusMsgFromCmd(tc.reload(m, path))
			if !ok {
				t.Fatal("the reload produced no status message")
			}
			if !strings.Contains(status.text, "Environment dev") {
				t.Fatalf("status = %q, want it to name what the reload activated", status.text)
			}
			if got := m.ws.active.Values()["token"]; got != "dev-token" {
				t.Fatalf("token = %q, want the file on disk to be active", got)
			}
			if cmd := m.runBlocked(); cmd != nil {
				t.Fatal("the session still refuses to send after the file loaded")
			}
		})
	}
}

func TestReloadingARequestFileLeavesTheEnvironmentAlone(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": loadedEnv,
		"api.http":         "GET http://example.test/\n",
	})
	model := modelInWorkspace(t, root, false)
	m := &model

	path := filepath.Join(root, "api.http")
	m.openFile(path)
	if err := os.WriteFile(path, []byte("GET http://example.test/{{token}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status, _ := statusMsgFromCmd(m.reloadFileFromDisk())
	if want := "Reloaded api.http"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
}

func TestSavingTheEnvironmentFileReplaysTheIntent(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": brokenEnv,
		"api.http":         "GET http://example.test/{{token}}\n",
	})
	cat, envFile, err := vars.Discover(root)
	if err == nil {
		t.Fatal("the environment file loaded, so there is nothing to recover from")
	}
	model := New(Config{
		Env: vars.Config{
			Catalog: cat,
			File:    envFile,
			FileErr: err,
			Intent:  vars.Intent{Name: "prod"},
		},
		WorkspaceRoot: root,
	})
	m := &model

	m.openFile(envFile)
	m.editor.SetValue(loadedEnv)
	status, _ := statusMsgFromCmd(m.saveFile())

	if want := "Saved resterm.env.json. Environment prod"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
	if got := m.ws.active.Values()["token"]; got != "prod-token" {
		t.Fatalf("token = %q, want the environment named at launch", got)
	}
}

func TestSavingTheEnvironmentFileWithoutTheSelectedEnvironment(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"resterm.env.json": loadedEnv})
	model := modelInWorkspace(t, root, false)
	m := &model
	if err := m.selectEnvironment("prod", nil); err != nil {
		t.Fatalf("select prod: %v", err)
	}

	m.openFile(filepath.Join(root, "resterm.env.json"))
	m.editor.SetValue(`{"dev":{"token":"dev-token"}}`)
	status, _ := statusMsgFromCmd(m.saveFile())

	if !strings.Contains(status.text, noEnvSelected) {
		t.Fatalf("status = %q, want %q", status.text, noEnvSelected)
	}
	if status.level != statusWarn {
		t.Fatalf("level = %v, want warn", status.level)
	}
	if !m.ws.unselected {
		t.Fatal("the session kept an environment the file no longer defines")
	}
}

func TestSavingABrokenEnvironmentFileKeepsTheLoadedOne(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"resterm.env.json": loadedEnv})
	model := modelInWorkspace(t, root, false)
	m := &model

	m.openFile(filepath.Join(root, "resterm.env.json"))
	m.editor.SetValue(brokenEnv)
	status, ok := statusMsgFromCmd(m.saveFile())
	if !ok {
		t.Fatal("save produced no status message")
	}

	if status.level != statusError {
		t.Fatalf("level = %v, want error", status.level)
	}
	for _, want := range []string{"Saved resterm.env.json", "Environment file failed to load"} {
		if !strings.Contains(status.text, want) {
			t.Fatalf("status = %q, want it to contain %q", status.text, want)
		}
	}
	if got := m.ws.active.Values()["token"]; got != "dev-token" {
		t.Fatalf("token = %q, want the loaded environment to stay active", got)
	}
	if cmd := m.runBlocked(); cmd != nil {
		t.Fatal("a session running on a loaded environment was stopped by a bad write")
	}
}

func TestSavingAStillBrokenEnvironmentFileKeepsRefusingToSend(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"resterm.env.json": brokenEnv})
	model := brokenEnvModel(t, root)
	m := &model

	m.openFile(filepath.Join(root, "resterm.env.json"))
	m.editor.SetValue(`{"dev":[]}`)
	status, _ := statusMsgFromCmd(m.saveFile())
	if !strings.Contains(status.text, "Environment file failed to load") {
		t.Fatalf("status = %q, want the new reason", status.text)
	}

	refused, ok := statusMsgFromCmd(m.runBlocked())
	if !ok {
		t.Fatal("the session sends on an environment file that never loaded")
	}
	if !strings.Contains(refused.text, `"dev" must be an object`) {
		t.Fatalf("refusal = %q, want the reason the last save failed with", refused.text)
	}
}

func TestSavingARequestFileLeavesTheEnvironmentAlone(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": loadedEnv,
		"api.http":         "GET http://example.test/\n",
	})
	model := modelInWorkspace(t, root, false)
	m := &model

	m.openFile(filepath.Join(root, "api.http"))
	m.editor.SetValue("GET http://example.test/{{token}}\n")
	status, _ := statusMsgFromCmd(m.saveFile())

	if want := "Saved api.http"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
}

func TestMovingWorkspaceKeepsABrokenPinnedEnvFile(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"api.http": "GET http://example.test/\n"})
	pinned := filepath.Join(
		writeWorkspace(t, map[string]string{"pinned.env.json": brokenEnv}),
		"pinned.env.json",
	)
	other := writeWorkspace(t, map[string]string{"resterm.env.json": loadedEnv})

	model := brokenPinnedEnvModel(t, root, pinned)
	m := &model

	m.applyOpenDirectory(other)

	if m.ws.envFile != pinned {
		t.Fatalf("envFile = %q, want the pinned %q", m.ws.envFile, pinned)
	}
	if cmd := m.runBlocked(); cmd == nil {
		t.Fatal("the move let the session send on an environment file that never loaded")
	}
}

func TestSavingAnEnvFileOutsideTheWorkspaceReloadsThatFile(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"resterm.env.json": `{"root":{"token":"root-token"}}`})
	pinned := filepath.Join(writeWorkspace(t, map[string]string{"pinned.env.json": brokenEnv}), "pinned.env.json")

	model := brokenPinnedEnvModel(t, root, pinned)
	m := &model

	m.openFile(pinned)
	m.editor.SetValue(loadedEnv)
	status, _ := statusMsgFromCmd(m.saveFile())

	if want := "Saved pinned.env.json. Environment dev"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
	if m.ws.envFile != pinned {
		t.Fatalf("envFile = %q, want %q", m.ws.envFile, pinned)
	}
	if got := m.ws.active.Values()["token"]; got != "dev-token" {
		t.Fatalf("token = %q, want the file the session runs on", got)
	}
}

func TestWorkspaceOwnsEnvFile(t *testing.T) {
	root := writeWorkspace(t, map[string]string{
		"resterm.env.json": loadedEnv,
		"pinned.env.json":  loadedEnv,
		"api.http":         "GET http://example.test/\n",
	})
	discovered := filepath.Join(root, "resterm.env.json")
	pinned := filepath.Join(root, "pinned.env.json")

	for _, tc := range []struct {
		name string
		ws   workspace
		path string
		want bool
	}{
		{
			name: "the discovered file",
			ws:   workspace{root: root, envFile: discovered},
			path: discovered,
			want: true,
		},
		{
			name: "a request file",
			ws:   workspace{root: root, envFile: discovered},
			path: filepath.Join(root, "api.http"),
		},
		{
			name: "the file passed with --env-file",
			ws:   workspace{root: root, envFile: pinned, envPinned: true},
			path: pinned,
			want: true,
		},
		{
			name: "a discoverable file beside the one in use",
			ws:   workspace{root: root, envFile: pinned, envPinned: true},
			path: discovered,
		},
		{
			name: "a file created after launch",
			ws:   workspace{root: root},
			path: discovered,
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ws.ownsEnvFile(tc.path); got != tc.want {
				t.Fatalf("ownsEnvFile(%s) = %t, want %t", filepath.Base(tc.path), got, tc.want)
			}
		})
	}
}

func TestSavingANewEnvironmentFileLoadsIt(t *testing.T) {
	root := writeWorkspace(t, map[string]string{"api.http": "GET http://example.test/\n"})
	model := modelInWorkspace(t, root, false)
	m := &model
	if !m.ws.cat.Empty() {
		t.Fatal("the workspace already has an environment file")
	}

	path := filepath.Join(root, "resterm.env.json")
	if err := os.WriteFile(path, []byte(loadedEnv), 0o644); err != nil {
		t.Fatal(err)
	}
	m.openFile(path)
	status, _ := statusMsgFromCmd(m.saveFile())

	if want := "Saved resterm.env.json. Environment dev"; status.text != want {
		t.Fatalf("status = %q, want %q", status.text, want)
	}
	if m.ws.envFile != path {
		t.Fatalf("envFile = %q, want %q", m.ws.envFile, path)
	}
}
