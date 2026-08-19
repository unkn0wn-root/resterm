package vars

// Config carries a session's environment from the command line into the UI in
// one piece, so a field added on one side cannot be forgotten on the other.
type Config struct {
	Catalog      Catalog
	Selection    Selection
	File         string
	FileExplicit bool
	Intent       Intent
	Fallback     string
	// FileErr holds a parse error that an editor session can recover from.
	FileErr error
}
