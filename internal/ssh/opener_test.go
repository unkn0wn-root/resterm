package ssh

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestSessionOpenerReportsRouteFailures(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	o := newSessionOpener(func(context.Context, execConfig) (sshClient, error) {
		return nil, refused
	}, 1)

	_, err := o.open(context.Background(), execConfig{Config: Config{Host: "bastion", Port: 22}}, false)
	if got := diag.Classes(err); !slices.Equal(got, []diag.Class{diag.ClassRoute}) {
		t.Fatalf("classes = %v, want only route: %v", got, err)
	}
}

func TestSessionOpenerKeepsSpecificClasses(t *testing.T) {
	missing := &fs.PathError{Op: "open", Path: "/home/u/.ssh/known_hosts", Err: fs.ErrNotExist}
	for name, want := range map[string]diag.Class{
		"canceled":     diag.ClassCanceled,
		"missing file": diag.ClassFilesystem,
	} {
		ctx, cancel := context.WithCancel(context.Background())
		o := newSessionOpener(func(ctx context.Context, _ execConfig) (sshClient, error) {
			if want == diag.ClassCanceled {
				cancel()
				return nil, ctx.Err()
			}
			return nil, missing
		}, 1)

		_, err := o.open(ctx, execConfig{Config: Config{Host: "bastion", Port: 22}}, false)
		if got := diag.ClassOf(err); got != want {
			t.Errorf("%s: class = %q, want %q: %v", name, got, want, err)
		}
		cancel()
	}
}
