//go:build darwin || linux

package fsx

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWriteInPlaceSizeLimit(t *testing.T) {
	if os.Getenv("RESTERM_TEST_FSIZE") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
		cmd.Env = append(os.Environ(), "RESTERM_TEST_FSIZE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	tests := []struct {
		name      string
		old, data int
		limit     uint64
	}{
		{"grow past the limit", 225, 325, 250},
		{"shrink past the limit", 325, 225, 16},
		{"same size past the limit", 325, 325, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "req.http")
			old := bytes.Repeat([]byte("o"), tt.old)
			if err := os.WriteFile(p, old, 0o644); err != nil {
				t.Fatal(err)
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
			err := WriteInPlace(p, bytes.Repeat([]byte("n"), tt.data), 0o644)
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &saved); err != nil {
				t.Fatal(err)
			}

			if !errors.Is(err, syscall.EFBIG) {
				t.Errorf("err = %v, want EFBIG", err)
			}
			got, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, old) {
				t.Errorf("file = %d bytes, %d new, want the old %d", len(got), bytes.Count(got, []byte("n")), len(old))
			}
		})
	}
}
