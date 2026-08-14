package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/prompt"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func TestCommandLineCompletesRequestPath(t *testing.T) {
	root := completionWorkspace(t)
	model := New(Config{WorkspaceRoot: root})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLine.input.SetValue("mock start --source a")

	model.handlePathRead(completionPathRead(t, model.commandLine.refresh(model.commandSource())))
	if labels := completionLabels(model.commandLine); !slices.Equal(labels, []string{"api" + pathSeparator}) {
		t.Fatalf("root suggestions = %q", labels)
	}

	nested := model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := model.commandLine.value(); got != "mock start --source api"+pathSeparator {
		t.Fatalf("directory completion = %q", got)
	}
	if nested == nil || !model.showCommandLine {
		t.Fatal("directory completion did not stay open and load its children")
	}

	model.handlePathRead(completionPathRead(t, nested))
	if labels := completionLabels(model.commandLine); !slices.Equal(labels, []string{"payments.rest"}) {
		t.Fatalf("nested suggestions = %q", labels)
	}

	model.commandLine.input.SetValue("mock start --source ../")
	if cmd := model.commandLine.refresh(model.commandSource()); cmd != nil || len(completionLabels(model.commandLine)) != 0 {
		t.Fatal("request completion escaped the workspace")
	}
}

func TestClosedCommandLineIgnoresPathRead(t *testing.T) {
	model := New(Config{WorkspaceRoot: completionWorkspace(t)})
	model.openCommandLine()
	model.commandLine.input.SetValue("mock start --source a")
	load := model.commandLine.refresh(model.commandSource())
	model.closeCommandLine()

	model.handlePathRead(completionPathRead(t, load))
	if labels := completionLabels(model.commandLine); len(labels) != 0 {
		t.Fatalf("closed command line took suggestions: %q", labels)
	}
}

func TestOpenModalListsItsDirectoryOnOpen(t *testing.T) {
	model := New(Config{WorkspaceRoot: completionWorkspace(t)})
	model.width = 100
	model.height = 32

	openPathModal(t, &model)

	want := []string{"api" + pathSeparator, "users.http"}
	if labels := completionLabels(model.openPathPrompt); !slices.Equal(labels, want) {
		t.Fatalf("suggestions on open = %q, want %q", labels, want)
	}
	if rendered := model.renderOpenModal(); !strings.Contains(rendered, "users.http") {
		t.Fatalf("opening frame drew no listing: %q", rendered)
	}
}

func TestOpenModalCompletesPaths(t *testing.T) {
	root := completionWorkspace(t)
	model := New(Config{WorkspaceRoot: root})
	model.width = 100
	model.height = 32
	openPathModal(t, &model)
	model.openPathPrompt.input.SetValue("a")
	model.openPathPrompt.refresh(model.openPathSource())

	if labels := completionLabels(model.openPathPrompt); !slices.Equal(labels, []string{"api" + pathSeparator}) {
		t.Fatalf("open suggestions = %q", labels)
	}
	if rendered := model.renderOpenModal(); !strings.Contains(rendered, "api") {
		t.Fatalf("open modal did not draw its suggestions: %q", rendered)
	}

	updated, nested := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if got := model.openPathPrompt.value(); got != "api"+pathSeparator {
		t.Fatalf("directory completion = %q", got)
	}
	if nested == nil || !model.showOpenModal {
		t.Fatal("Tab did not keep the modal open and load the directory")
	}
}

func TestEditCommandOpensQuotedPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "space dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "demo file.http")
	if err := os.WriteFile(path, []byte("GET https://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := New(Config{WorkspaceRoot: root})

	collectMsgs(model.executeExCommand(`edit "space dir/demo file.http"`))
	if !util.SamePath(model.currentFile, path) {
		t.Fatalf("current file = %q, want %q", model.currentFile, path)
	}
}

var pathSeparator = string(filepath.Separator)

func TestListDirSoonDeliversAFastListing(t *testing.T) {
	load := prompt.DirLoad{Dir: "api", Gen: 7}
	listing := prompt.DirRead{DirLoad: load, Entries: []prompt.DirEntry{{Name: "users.http"}}}

	read, later := listDirSoon(promptOpenPath, load, func(prompt.DirLoad) prompt.DirRead {
		return listing
	}, time.Minute)

	if later != nil {
		t.Fatal("a listing that finished in time went back to the update loop")
	}
	if len(read.Entries) != 1 || read.Entries[0].Name != "users.http" {
		t.Fatalf("delivered read = %+v", read)
	}
}

func TestListDirSoonHandsASlowListingToTheUpdateLoop(t *testing.T) {
	load := prompt.DirLoad{Dir: "api", Gen: 7}
	release := make(chan struct{})
	slow := func(prompt.DirLoad) prompt.DirRead {
		<-release
		return prompt.DirRead{DirLoad: load, Entries: []prompt.DirEntry{{Name: "users.http"}}}
	}

	read, later := listDirSoon(promptOpenPath, load, slow, time.Millisecond)
	if later == nil {
		t.Fatal("a listing past the wait did not go back to the update loop")
	}
	if len(read.Entries) != 0 {
		t.Fatalf("a listing still running was delivered inline: %+v", read)
	}

	close(release)
	msg, ok := later().(pathReadMsg)
	if !ok {
		t.Fatalf("deferred listing produced %T", msg)
	}
	if msg.id != promptOpenPath || len(msg.read.Entries) != 1 {
		t.Fatalf("deferred listing = %+v", msg)
	}
}

// The prompt lists its first directory inline when it can, so drain the read
// for the times it cannot.
func openPathModal(t *testing.T, model *Model) {
	t.Helper()
	if read, ok := findCompletionPathRead(model.openOpenModal()); ok {
		model.handlePathRead(read)
	}
}

func completionLabels(p completionPrompt) []string {
	items := p.menu.Items()
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i] = item.Label
	}
	return labels
}

func completionPathRead(t *testing.T, cmd tea.Cmd) pathReadMsg {
	t.Helper()
	msg, ok := findCompletionPathRead(cmd)
	if !ok {
		t.Fatal("no directory read was scheduled")
	}
	return msg
}

func findCompletionPathRead(cmd tea.Cmd) (pathReadMsg, bool) {
	if cmd == nil {
		return pathReadMsg{}, false
	}
	switch msg := cmd().(type) {
	case pathReadMsg:
		return msg, true
	case tea.BatchMsg:
		for _, nested := range msg {
			if read, ok := findCompletionPathRead(nested); ok {
				return read, true
			}
		}
	}
	return pathReadMsg{}, false
}

func completionWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"users.http",
		"notes.txt",
		filepath.Join("api", "payments.rest"),
	} {
		if err := os.WriteFile(filepath.Join(root, path), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
