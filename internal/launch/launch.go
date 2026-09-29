// Package launch opens URLs and files with the platform's default application.
package launch

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

const openGrace = 2 * time.Second

type process interface {
	Wait() error
}

type startFunc func(name string, args ...string) (process, error)

// Launcher starts platform applications without invoking a shell on macOS or Linux.
type Launcher struct {
	goos  string
	start startFunc
	grace time.Duration
}

func New() Launcher {
	return Launcher{goos: runtime.GOOS, start: startCommand, grace: openGrace}
}

// Open only reports errors from openers that exit within openGrace. Without a
// desktop environment xdg-open runs the app itself and exits when the app
// closes, so after openGrace we stop waiting and let a goroutine collect it.
func (l Launcher) Open(target string) error {
	name, args := "xdg-open", []string{target}
	switch l.goos {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	}

	p, err := l.start(name, args...)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	case <-time.After(l.grace):
		return nil
	}
}

// Leaving Stderr nil sends it to os.DevNull. A pipe would be inherited by the
// app the opener starts, and that app dies of SIGPIPE once the pipe closes.
func startCommand(name string, args ...string) (process, error) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
