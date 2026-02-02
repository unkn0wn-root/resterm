package initcmd

import "io/fs"

const (
	DefaultDir      = "."
	DefaultTemplate = "standard"
)

const (
	fileRequests   = "requests.http"
	fileEnv        = "resterm.env.json"
	fileEnvExample = "resterm.env.example.json"
	fileHelp       = "RESTERM.md"
	fileRTSHelpers = "rts/helpers.rts"
	gitignoreFile  = ".gitignore"
	gitignoreEntry = fileEnv
)

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
)
