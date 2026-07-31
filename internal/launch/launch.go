// Package launch opens URLs and files with the platform's default application.
package launch

import (
	"os/exec"
	"runtime"
)

type startFunc func(name string, args ...string) error

// Launcher starts platform applications without invoking a shell on macOS or Linux.
type Launcher struct {
	goos  string
	start startFunc
}

func New() Launcher {
	return Launcher{goos: runtime.GOOS, start: startCommand}
}

func (l Launcher) Open(target string) error {
	switch l.goos {
	case "darwin":
		return l.start("open", target)
	case "windows":
		return l.start("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		return l.start("xdg-open", target)
	}
}

func startCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}
