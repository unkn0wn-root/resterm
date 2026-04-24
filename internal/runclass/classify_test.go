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

func TestClassifyMessageSharedRules(t *testing.T) {
	classifiers := []struct {
		name string
		fn   func(string, string) Failure
	}{
		{name: "generic", fn: classifyMessage},
		{name: "http", fn: classifyHTTPMessage},
	}
	for _, classifier := range classifiers {
		t.Run(classifier.name, func(t *testing.T) {
			got := classifier.fn("oauth token expired", "source")
			if got.Code != FailureAuth {
				t.Fatalf("classification = %+v, want code=%s", got, FailureAuth)
			}
		})
	}
}

func TestClassifyMessageContextSpecificRules(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string, string) Failure
		msg  string
		code FailureCode
	}{
		{
			name: "generic default unknown",
			fn:   classifyMessage,
			msg:  "unexpected failure",
			code: FailureUnknown,
		},
		{
			name: "generic profile is not filesystem",
			fn:   classifyMessage,
			msg:  "profile data unavailable",
			code: FailureUnknown,
		},
		{
			name: "generic mainstream is not protocol",
			fn:   classifyMessage,
			msg:  "mainstream response unavailable",
			code: FailureUnknown,
		},
		{
			name: "http default protocol",
			fn:   classifyHTTPMessage,
			msg:  "unexpected failure",
			code: FailureProtocol,
		},
		{
			name: "generic filesystem",
			fn:   classifyMessage,
			msg:  "open config: permission denied",
			code: FailureFilesystem,
		},
		{
			name: "generic standalone file",
			fn:   classifyMessage,
			msg:  "file not found",
			code: FailureFilesystem,
		},
		{
			name: "http proxy network",
			fn:   classifyHTTPMessage,
			msg:  "proxy connection reset",
			code: FailureNetwork,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn(tc.msg, "source")
			if got.Code != tc.code {
				t.Fatalf("classification = %+v, want code=%s", got, tc.code)
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
	if got := ReportExitCode(failures, true, ExitCodeModeSummary); got != ExitFailure {
		t.Fatalf("summary exit code = %d, want %d", got, ExitFailure)
	}
}
