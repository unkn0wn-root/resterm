package ui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/prompt"
)

// Save the revision and caret position to reject directory reads after an edit or move.
type editorPathCompletion struct {
	session     prompt.PathSession
	root        string
	ctx         intellisense.Context
	lineOffset  int
	revision    uint64
	caretOffset int
}

func (p *editorPathCompletion) reset() {
	p.session.Reset()
	p.ctx = intellisense.Context{}
}

// SetCompletionRoot sets the base directory for relative path suggestions.
func (e *requestEditor) SetCompletionRoot(dir string) {
	dir = filepath.Clean(dir)
	if e.paths.root == dir {
		return
	}
	e.closeCompletions()
	e.paths.root = dir
}

func (e *requestEditor) showPathCompletions(ctx intellisense.Context, line int) tea.Cmd {
	items, load := e.paths.session.SuggestPath(prompt.PathRequest{
		Value:  ctx.Query,
		Suffix: ctx.Path.Suffix,
		Edit:   prompt.Edit{Start: ctx.Start, End: ctx.End},
		Spec:   editorPathSpec(e.paths.root, ctx.Path),
	})
	e.paths.ctx = ctx
	e.paths.lineOffset = e.offsetForPosition(line, 0)
	e.paths.revision = e.Revision()
	e.paths.caretOffset = e.caretPosition().Offset
	e.setPathCompletionItems(items)
	if !load.Pending() {
		return nil
	}
	return readPathDir(promptEditor, load)
}

func editorPathSpec(root string, ctx intellisense.PathContext) prompt.PathSpec {
	filter, summary := files.AnyPathFilter(), "file"
	switch ctx.Kind {
	case intellisense.PathRTS:
		filter, summary = files.KindPathFilter(files.KindScript), "RestermScript module"
	case intellisense.PathGraphQL:
		filter, summary = files.KindPathFilter(files.KindGraphQL), "GraphQL file"
	case intellisense.PathJSON:
		filter, summary = files.KindPathFilter(files.KindJSON), "JSON file"
	case intellisense.PathScript:
		filter, summary = files.KindPathFilter(files.KindScript, files.KindJavaScript), "script file"
	}
	separators := prompt.SeparatorNone
	if ctx.List {
		separators = prompt.SeparatorPathList
	}
	return prompt.PathSpec{
		Root:        root,
		Files:       filter,
		FileSummary: summary,
		Separators:  separators,
		ExpandHome:  ctx.Home,
		Quote:       ctx.Quote,
	}
}

func (e *requestEditor) deliverPathCompletion(read prompt.DirRead) {
	if !e.completionEnabled || e.paths.revision != e.Revision() ||
		e.paths.caretOffset != e.caretPosition().Offset {
		e.paths.reset()
	}
	items, ok := e.paths.session.Deliver(read)
	if !ok {
		return
	}
	e.setPathCompletionItems(items)
}

func (e *requestEditor) setPathCompletionItems(items []prompt.Item) {
	ctx := e.paths.ctx
	converted := make([]intellisense.Item, 0, len(items))
	for _, item := range items {
		out := intellisense.Item{
			Label:      item.Label,
			Summary:    item.Summary,
			Insert:     item.Edit.Text,
			CursorBack: item.Edit.CursorBack,
			Continue:   item.Continue,
		}
		if ctx.Path.Continues && item.Edit.CursorBack == 0 {
			out.Continue = true
		}
		// Add no space while entering a directory or completing the rest of the line.
		if item.Continue || !ctx.Path.Quote {
			out = out.WithoutTrailingSpace()
		}
		converted = append(converted, out)
	}
	e.completion.update(e.paths.lineOffset+ctx.Start, converted, ctx)
}
