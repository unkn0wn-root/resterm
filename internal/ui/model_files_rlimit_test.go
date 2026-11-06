//go:build darwin || linux

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSaveFileFailureKeepsBuffer(t *testing.T) {
	if os.Getenv("RESTERM_TEST_FSIZE") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
		cmd.Env = append(os.Environ(), "RESTERM_TEST_FSIZE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	small := "GET https://small.example/" + strings.Repeat("a", 200) + "\n"
	big := "GET https://big.example/" + strings.Repeat("b", 300) + "\n"
	outside := "GET https://outside.example\n"
	tests := []struct {
		name            string
		open, disk, buf string
		limit           uint64
		dirty           bool
	}{
		{"dirty buffer grows past the limit", small, small, big, 250, true},
		{"clean buffer over the limit", big, big, big, 16, true},
		{"clean buffer after an outside write", small, outside, small, 16, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "req.http")
			if err := os.WriteFile(path, []byte(tt.open), 0o644); err != nil {
				t.Fatal(err)
			}
			model := New(Config{WorkspaceRoot: dir, FilePath: path, InitialContent: tt.open})
			m := &model
			if tt.buf != tt.open {
				m.editor.SetValue(tt.buf)
				m.markDirty()
			}
			if tt.disk != tt.open {
				if err := os.WriteFile(path, []byte(tt.disk), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var lim syscall.Rlimit
			if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
				t.Fatal(err)
			}
			saved := lim
			lim.Cur = tt.limit
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lim); err != nil {
				t.Fatal(err)
			}
			status, _ := statusMsgFromCmd(m.saveFile())
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &saved); err != nil {
				t.Fatal(err)
			}

			if status.level != statusError || !strings.HasPrefix(status.text, "save failed: ") {
				t.Fatalf("status = %q (level %v), want a save error", status.text, status.level)
			}
			if got := m.editor.Value(); got != tt.buf {
				t.Errorf("buffer = %q, want %q", got, tt.buf)
			}
			if m.dirty != tt.dirty {
				t.Errorf("dirty = %v, want %v", m.dirty, tt.dirty)
			}
			if msg, ok := scanFileWatch(m); ok {
				t.Errorf("watcher reported kind %v for our own failed save", msg.kind)
			}
			if quit := commandHasQuit(m.executeExCommand("q")); quit == tt.dirty {
				t.Errorf(":q quit = %v with dirty %v", quit, tt.dirty)
			}

			if err := os.WriteFile(path, []byte("GET https://later.example\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, ok := scanFileWatch(m); !ok {
				t.Error("watcher missed an outside write after the failed save")
			}
		})
	}
}
