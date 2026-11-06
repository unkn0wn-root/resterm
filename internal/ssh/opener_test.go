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
	closed := &net.OpError{Op: "close", Net: "unix", Err: errors.New("broken pipe")}
	tests := []struct {
		cause error
		want  string
	}{
		{refused, "error[route]: open ssh tunnel to bastion:22: dial tcp: connection refused"},
		{
			errors.Join(errors.New("ssh: handshake failed: EOF"), closed),
			"error[route]: open ssh tunnel to bastion:22: ssh: handshake failed: EOF; close unix: broken pipe",
		},
		{errors.New("ssh: banner\nsecond"), `error[route]: open ssh tunnel to bastion:22: ssh: banner\nsecond`},
	}
	for _, tt := range tests {
		o := newSessionOpener(func(context.Context, execConfig) (sshClient, error) {
			return nil, tt.cause
		}, 1)

		_, err := o.open(context.Background(), execConfig{Config: Config{Host: "bastion", Port: 22}}, false)
		if got := diag.Classes(err); !slices.Equal(got, []diag.Class{diag.ClassRoute}) {
			t.Errorf("classes = %v, want only route: %v", got, err)
		}
		if got := diag.Render(err); got != tt.want {
			t.Errorf("Render() = %q, want %q", got, tt.want)
		}
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
