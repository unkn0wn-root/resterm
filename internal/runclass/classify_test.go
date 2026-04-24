package runclass

import (
	"context"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

func TestClassifyErrorTypedFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code FailureCode
		exit int
	}{
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			code: FailureTimeout,
			exit: ExitTimeout,
		},
		{
			name: "network",
			err:  &net.DNSError{Err: "no such host", Name: "api.local"},
			code: FailureNetwork,
			exit: ExitNetwork,
		},
		{
			name: "tls",
			err:  x509.UnknownAuthorityError{},
			code: FailureTLS,
			exit: ExitTLS,
		},
		{
			name: "filesystem",
			err:  &fs.PathError{Op: "open", Path: "config.env", Err: fs.ErrNotExist},
			code: FailureFilesystem,
			exit: ExitFilesystem,
		},
		{
			name: "script",
			err:  errdef.Wrap(errdef.CodeScript, errors.New("boom"), "pre-request"),
			code: FailureScript,
			exit: ExitScript,
		},
		{
			name: "errdef timeout",
			err:  errdef.New(errdef.CodeTimeout, "operation timed out"),
			code: FailureTimeout,
			exit: ExitTimeout,
		},
		{
			name: "errdef canceled",
			err:  errdef.New(errdef.CodeCanceled, "operation canceled"),
			code: FailureCanceled,
			exit: ExitCanceled,
		},
		{
			name: "errdef network",
			err:  errdef.New(errdef.CodeNetwork, "network unavailable"),
			code: FailureNetwork,
			exit: ExitNetwork,
		},
		{
			name: "errdef tls",
			err:  errdef.New(errdef.CodeTLS, "certificate rejected"),
			code: FailureTLS,
			exit: ExitTLS,
		},
		{
			name: "errdef auth",
			err:  errdef.New(errdef.CodeAuth, "token rejected"),
			code: FailureAuth,
			exit: ExitAuth,
		},
		{
			name: "errdef protocol",
			err:  errdef.New(errdef.CodeProtocol, "invalid frame"),
			code: FailureProtocol,
			exit: ExitProtocol,
		},
		{
			name: "errdef route",
			err:  errdef.New(errdef.CodeRoute, "tunnel unavailable"),
			code: FailureRoute,
			exit: ExitRoute,
		},
		{
			name: "joined errdefs choose dominant code",
			err: errors.Join(
				errdef.New(errdef.CodeAuth, "token rejected"),
				errdef.New(errdef.CodeTimeout, "command timed out"),
			),
			code: FailureTimeout,
			exit: ExitTimeout,
		},
		{
			name: "http wrapper defers to nested code",
			err: errdef.Wrap(
				errdef.CodeHTTP,
				errdef.New(errdef.CodeFilesystem, "body unavailable"),
				"read request body",
			),
			code: FailureFilesystem,
			exit: ExitFilesystem,
		},
		{
			name: "http wrapper uses http fallback",
			err:  errdef.New(errdef.CodeHTTP, "proxy connection reset"),
			code: FailureNetwork,
			exit: ExitNetwork,
		},
		{
			name: "wrapped errdef timeout wins over auth wrapper",
			err: errdef.Wrap(
				errdef.CodeAuth,
				errdef.New(errdef.CodeTimeout, "command timed out"),
				"resolve command auth",
			),
			code: FailureTimeout,
			exit: ExitTimeout,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.err)
			if got.Code != tc.code || got.ExitCode != tc.exit {
				t.Fatalf("ClassifyError = %+v, want code=%s exit=%d", got, tc.code, tc.exit)
			}
		})
	}
}

func TestReportExitCodeModes(t *testing.T) {
	failures := []Failure{
		AssertionFailure("test failed", "tests"),
		NewFailure(FailureTimeout, "deadline exceeded", "error"),
	}
	if got := ReportExitCode(failures, true, ExitCodeModeDetailed); got != ExitTimeout {
		t.Fatalf("detailed exit code = %d, want %d", got, ExitTimeout)
	}
	if got := ReportExitCode(failures, true, ""); got != ExitTimeout {
		t.Fatalf("default exit code = %d, want %d", got, ExitTimeout)
	}
	if got := ReportExitCode(failures, true, ExitCodeModeSummary); got != ExitFailure {
		t.Fatalf("summary exit code = %d, want %d", got, ExitFailure)
	}
}
