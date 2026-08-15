package ui

import (
	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/prompt"
)

type commandSource struct {
	catalog   exCatalog
	mockRoot  string
	workspace string
	envFile   string
}

func (s commandSource) PathAt(input string, cursor int) (prompt.PathRequest, bool) {
	arg, ok := s.catalog.pathArg(input, cursor)
	if !ok {
		return prompt.PathRequest{}, false
	}

	spec, ok := s.spec(arg.kind)
	if !ok {
		return prompt.PathRequest{}, false
	}
	// Path arguments may contain whitespace, so quote them when needed.
	spec.Quote = true
	return prompt.PathRequest{Value: arg.value, Edit: arg.edit, Spec: spec}, true
}

func (s commandSource) Suggest(input string) []prompt.Item {
	return s.catalog.Suggestions(input)
}

func (s commandSource) spec(kind pathArgKind) (prompt.PathSpec, bool) {
	switch kind {
	case pathArgWorkspace:
		return workspacePathSpec(s.workspace, s.envFile), true
	case pathArgRequests:
		return requestPathSpec(s.mockRoot), true
	default:
		return prompt.PathSpec{}, false
	}
}

type openPathSource struct {
	workspace string
	envFile   string
}

func (s openPathSource) PathAt(input string, cursor int) (prompt.PathRequest, bool) {
	runes := []rune(input)
	if cursor < 0 || cursor > len(runes) {
		return prompt.PathRequest{}, false
	}
	return prompt.PathRequest{
		Value: string(runes[:cursor]),
		Edit:  prompt.Edit{End: len(runes)},
		Spec:  workspacePathSpec(s.workspace, s.envFile),
	}, true
}

func (openPathSource) Suggest(string) []prompt.Item { return nil }

// responseSavePathSource accepts any file and uses directories for navigation.
type responseSavePathSource struct {
	dir string
}

func (s responseSavePathSource) PathAt(input string, cursor int) (prompt.PathRequest, bool) {
	runes := []rune(input)
	if cursor < 0 || cursor > len(runes) {
		return prompt.PathRequest{}, false
	}
	return prompt.PathRequest{
		Value: string(runes[:cursor]),
		Edit:  prompt.Edit{End: len(runes)},
		Spec: prompt.PathSpec{
			Root:        s.dir,
			Files:       files.AnyPathFilter(),
			FileSummary: "file",
			ExpandHome:  true,
		},
	}, true
}

func (responseSavePathSource) Suggest(string) []prompt.Item { return nil }

var (
	_ completionSource = commandSource{}
	_ completionSource = openPathSource{}
	_ completionSource = responseSavePathSource{}
)

func (m *Model) commandSource() commandSource {
	return commandSource{
		catalog:   exCommands,
		mockRoot:  m.mockRoot(),
		workspace: m.ws.root,
		envFile:   m.ws.envFile,
	}
}

func (m *Model) openPathSource() openPathSource {
	return openPathSource{workspace: m.ws.root, envFile: m.ws.envFile}
}

func (m *Model) responseSavePathSource() responseSavePathSource {
	return responseSavePathSource{dir: m.responseSaveDir()}
}

func workspacePathSpec(root, envFile string) prompt.PathSpec {
	return prompt.PathSpec{
		Root:        root,
		Files:       files.WorkspacePathFilter(envFile),
		FileSummary: "workspace file",
		AcceptDirs:  true,
		ExpandHome:  true,
	}
}

func requestPathSpec(root string) prompt.PathSpec {
	return prompt.PathSpec{
		Root:        root,
		Files:       files.RequestPathFilter(),
		FileSummary: "request file",
		Confine:     true,
		CommaList:   true,
	}
}
