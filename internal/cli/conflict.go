package cli

import (
	"fmt"
	"os"
)

func HasFileConflict(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func CommandFileConflict(app, name, hint string) error {
	return fmt.Errorf(
		"%s: found file named %q in the current directory; use `%s -- %s` or `%s ./%s` to open it, or %s",
		name,
		name,
		app,
		name,
		app,
		name,
		hint,
	)
}
