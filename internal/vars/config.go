package vars

// Config carries a session's environment from the command line into the UI in
// one piece, so a field added on one side cannot be forgotten on the other.
type Config struct {
	Catalog   Catalog
	Selection Selection
	// File is the environment file the catalog was read from, empty when none
	// was found. FileExplicit marks a file named on the command line, which is
	// pinned for the session rather than owned by one workspace.
	File         string
	FileExplicit bool
	Intent       Intent
	// Fallback labels the environment picked by default when several were
	// available, for the startup status line.
	Fallback string
}
