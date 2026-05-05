package runfail

import (
	"context"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
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
			err:  diag.WrapAs(diag.ClassScript, errors.New("boom"), "pre-request"),
			code: CodeScript,
			exit: ExitScript,
		},
		{
			name: "diag timeout",
			err:  diag.New(diag.ClassTimeout, "operation timed out"),
			code: CodeTimeout,
			exit: ExitTimeout,
		},
		{
			name: "diag canceled",
			err:  diag.New(diag.ClassCanceled, "operation canceled"),
			code: CodeCanceled,
			exit: ExitCanceled,
		},
		{
			name: "diag network",
			err:  diag.New(diag.ClassNetwork, "network unavailable"),
			code: CodeNetwork,
			exit: ExitNetwork,
		},
		{
			name: "diag tls",
			err:  diag.New(diag.ClassTLS, "certificate rejected"),
			code: CodeTLS,
			exit: ExitTLS,
		},
		{
			name: "diag auth",
			err:  diag.New(diag.ClassAuth, "token rejected"),
			code: CodeAuth,
			exit: ExitAuth,
		},
		{
			name: "diag protocol",
			err:  diag.New(diag.ClassProtocol, "invalid frame"),
			code: CodeProtocol,
			exit: ExitProtocol,
		},
		{
			name: "diag route",
			err:  diag.New(diag.ClassRoute, "tunnel unavailable"),
			code: CodeRoute,
			exit: ExitRoute,
		},
		{
			name: "joined diags choose dominant code",
			err: errors.Join(
				diag.New(diag.ClassAuth, "token rejected"),
				diag.New(diag.ClassTimeout, "command timed out"),
			),
			code: CodeTimeout,
			exit: ExitTimeout,
		},
		{
			name: "http component wrapper defers to nested class",
			err: diag.Wrap(
				diag.New(diag.ClassFilesystem, "body unavailable"),
				"read request body",
				diag.WithComponent(diag.ComponentHTTP),
			),
			code: CodeFilesystem,
			exit: ExitFilesystem,
		},
		{
			name: "plain message uses network fallback",
			err:  errors.New("proxy connection reset"),
			code: CodeNetwork,
			exit: ExitNetwork,
		},
		{
			name: "wrapped diag timeout wins over auth wrapper",
			err: diag.WrapAs(diag.ClassAuth, diag.New(diag.ClassTimeout, "command timed out"),
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
