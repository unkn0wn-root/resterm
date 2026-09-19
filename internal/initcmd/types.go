package initcmd

import "io/fs"

type fileSpec struct {
	Path string
	Data string
	Mode fs.FileMode
}

// template represents a named starter set that can write multiple files.
// AddGitignore controls whether resterm.env.json is added to .gitignore.
type template struct {
	Name         string
	Description  string
	Files        []fileSpec
	AddGitignore bool
}

type Action string

const (
	ActionCreate    Action = "create"
	ActionOverwrite Action = "overwrite"
	ActionAppend    Action = "append"
	ActionSkip      Action = "skip"
)

type op struct {
	Action Action
	Path   string // relative (for reporting)
	Abs    string // absolute target
	Mode   fs.FileMode
	Data   string
	Prev   *prior // file being replaced, nil when creating
}

// prior is kept so a failed commit can put the original file back.
type prior struct {
	Data string
	Mode fs.FileMode
}
