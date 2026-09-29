package launch

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type fakeProcess struct {
	wait func() error
}

func (p fakeProcess) Wait() error {
	if p.wait == nil {
		return nil
	}
	return p.wait()
}

func TestLauncherOpen(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		target   string
		expected []string
	}{
		{
			name: "macos url", goos: "darwin", target: "https://example.com/docs",
			expected: []string{"open", "https://example.com/docs"},
		},
		{
			name: "windows url", goos: "windows", target: "https://example.com/docs",
			expected: []string{"rundll32", "url.dll,FileProtocolHandler", "https://example.com/docs"},
		},
		{
			name: "linux url", goos: "linux", target: "https://example.com/docs",
			expected: []string{"xdg-open", "https://example.com/docs"},
		},
		{
			name: "macos file", goos: "darwin", target: "/tmp/response.json",
			expected: []string{"open", "/tmp/response.json"},
		},
		{
			name: "windows file", goos: "windows", target: "/tmp/response.json",
			expected: []string{"rundll32", "url.dll,FileProtocolHandler", "/tmp/response.json"},
		},
		{
			name: "linux file", goos: "linux", target: "/tmp/response.json",
			expected: []string{"xdg-open", "/tmp/response.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			l := Launcher{
				goos: tt.goos,
				start: func(name string, args ...string) (process, error) {
					got = append([]string{name}, args...)
					return fakeProcess{}, nil
				},
				grace: time.Second,
			}
			if err := l.Open(tt.target); err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("Open() command = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLauncherReturnsStartError(t *testing.T) {
	want := errors.New("start failed")
	l := Launcher{
		goos:  "linux",
		start: func(string, ...string) (process, error) { return nil, want },
		grace: time.Second,
	}
	if got := l.Open("https://example.com"); !errors.Is(got, want) {
		t.Fatalf("Open() error = %v, want %v", got, want)
	}
}

func TestLauncherReturnsOpenerFailure(t *testing.T) {
	want := errors.New("exit status 1")
	l := Launcher{
		goos: "darwin",
		start: func(string, ...string) (process, error) {
			return fakeProcess{wait: func() error { return want }}, nil
		},
		grace: time.Second,
	}
	got := l.Open("/tmp/response.bin")
	if !errors.Is(got, want) || !strings.HasPrefix(got.Error(), "open: ") {
		t.Fatalf("Open() error = %v, want %v from open", got, want)
	}
}

func TestLauncherReapsOpenerAfterGrace(t *testing.T) {
	release := make(chan struct{})
	reaped := make(chan struct{})
	l := Launcher{
		goos: "linux",
		start: func(string, ...string) (process, error) {
			return fakeProcess{wait: func() error {
				<-release
				close(reaped)
				return errors.New("viewer closed")
			}}, nil
		},
		grace: 10 * time.Millisecond,
	}
	if err := l.Open("/tmp/response.pdf"); err != nil {
		t.Fatalf("Open() error = %v, want nil once the grace period ends", err)
	}
	close(release)
	select {
	case <-reaped:
	case <-time.After(time.Second):
		t.Fatal("opener was not waited on after the grace period")
	}
}

func TestStartCommandLeavesChildrenAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	marker := filepath.Join(t.TempDir(), "done")
	p, err := startCommand("sh", "-c", "(sleep 2; echo late >&2; echo ok > "+marker+") & exit 0")
	if err != nil {
		t.Fatalf("startCommand() error = %v", err)
	}
	start := time.Now()
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Wait() took %v, want it to return when the opener exits", d)
	}
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
	}
	t.Fatal("child died after writing to stderr")
}
