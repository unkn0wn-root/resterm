package initcmd

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
	dirPerm  = 0o755
	filePerm = 0o644
)

const (
	actionCreate    = "create"
	actionOverwrite = "overwrite"
	actionAppend    = "append"
	actionSkip      = "skip"
)
