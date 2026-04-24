package runfail

import (
	"context"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

func TestFromErrorTypedFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code Code
		exit int
	}{
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			code: CodeTimeout,
			exit: ExitTimeout,
		},
		{
			name: "network",
			err:  &net.DNSError{Err: "no such host", Name: "api.local"},
			code: CodeNetwork,
			exit: ExitNetwork,
		},
		{
			name: "tls",
			err:  x509.UnknownAuthorityError{},
			code: CodeTLS,
			exit: ExitTLS,
		},
		{
			name: "filesystem",
			err:  &fs.PathError{Op: "open", Path: "config.env", Err: fs.ErrNotExist},
			code: CodeFilesystem,
			exit: ExitFilesystem,
		},
		{
			name: "script",
			err:  errdef.Wrap(errdef.CodeScript, errors.New("boom"), "pre-request"),
			code: CodeScript,
			exit: ExitScript,
		},
		{
			name: "errdef timeout",
			err:  errdef.New(errdef.CodeTimeout, "operation timed out"),
			code: CodeTimeout,
			exit: ExitTimeout,
		},
		{
			name: "errdef canceled",
			err:  errdef.New(errdef.CodeCanceled, "operation canceled"),
			code: CodeCanceled,
			exit: ExitCanceled,
		},
		{
			name: "errdef network",
			err:  errdef.New(errdef.CodeNetwork, "network unavailable"),
			code: CodeNetwork,
			exit: ExitNetwork,
		},
		{
			name: "errdef tls",
			err:  errdef.New(errdef.CodeTLS, "certificate rejected"),
			code: CodeTLS,
			exit: ExitTLS,
		},
		{
			name: "errdef auth",
			err:  errdef.New(errdef.CodeAuth, "token rejected"),
			code: CodeAuth,
			exit: ExitAuth,
		},
		{
			name: "errdef protocol",
			err:  errdef.New(errdef.CodeProtocol, "invalid frame"),
			code: CodeProtocol,
			exit: ExitProtocol,
		},
		{
			name: "errdef route",
			err:  errdef.New(errdef.CodeRoute, "tunnel unavailable"),
			code: CodeRoute,
			exit: ExitRoute,
		},
		{
			name: "joined errdefs choose dominant code",
			err: errors.Join(
				errdef.New(errdef.CodeAuth, "token rejected"),
				errdef.New(errdef.CodeTimeout, "command timed out"),
			),
			code: CodeTimeout,
			exit: ExitTimeout,
		},
		{
			name: "http wrapper defers to nested code",
			err: errdef.Wrap(
				errdef.CodeHTTP,
				errdef.New(errdef.CodeFilesystem, "body unavailable"),
				"read request body",
			),
			code: CodeFilesystem,
			exit: ExitFilesystem,
		},
		{
			name: "http wrapper uses http fallback",
			err:  errdef.New(errdef.CodeHTTP, "proxy connection reset"),
			code: CodeNetwork,
			exit: ExitNetwork,
		},
		{
			name: "wrapped errdef timeout wins over auth wrapper",
			err: errdef.Wrap(
				errdef.CodeAuth,
				errdef.New(errdef.CodeTimeout, "command timed out"),
				"resolve command auth",
			),
			code: CodeTimeout,
			exit: ExitTimeout,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FromError(tc.err)
			if got.Code != tc.code || got.ExitCode != tc.exit {
				t.Fatalf("FromError = %+v, want code=%s exit=%d", got, tc.code, tc.exit)
			}
		})
	}
}

func TestCatalogCoversKnownCodes(t *testing.T) {
	for _, code := range KnownCodes() {
		m, ok := Lookup(code)
		if !ok {
			t.Fatalf("missing catalog entry for %q", code)
		}
		if m.Category == "" || m.ExitCode == 0 || m.Rank < 0 {
			t.Fatalf("incomplete catalog entry for %q: %+v", code, m)
		}
		got := New(code, "message", "source")
		if got.Code != code || got.Category != m.Category || got.ExitCode != m.ExitCode {
			t.Fatalf("New(%q) = %+v, want catalog %+v", code, got, m)
		}
	}
}

func TestReportExitCodeModes(t *testing.T) {
	failures := []Failure{
		Assertion("test failed", "tests"),
		New(CodeTimeout, "deadline exceeded", "error"),
	}
	if got := ExitCode(failures, true, ExitDetailed); got != ExitTimeout {
		t.Fatalf("detailed exit code = %d, want %d", got, ExitTimeout)
	}
	if got := ExitCode(failures, true, ""); got != ExitTimeout {
		t.Fatalf("default exit code = %d, want %d", got, ExitTimeout)
	}
	if got := ExitCode(failures, true, ExitSummary); got != ExitFailure {
		t.Fatalf("summary exit code = %d, want %d", got, ExitFailure)
	}
}
