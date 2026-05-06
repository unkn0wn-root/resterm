package diag_test

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestRenderReportWithSourceSpan(t *testing.T) {
	err := diag.FromReport(diag.Report{
		Path:   "sample.http",
		Source: []byte("GET https://example.com\n# @rts pre\n"),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassParse,
			Severity: diag.SeverityError,
			Message:  "@rts supports only pre-request mode",
			Span: diag.Span{Start: diag.Pos{
				Line: 2,
				Col:  3,
			}},
			Notes: []diag.Note{{
				Kind:    diag.NoteInfo,
				Message: "Fix the request file parse error before running.",
			}},
		}},
	}, nil)

	got := diag.Render(err)
	for _, want := range []string{
		"error[parse]: @rts supports only pre-request mode",
		"--> sample.http:2:3",
		"   2 | # @rts pre",
		"     |   ^",
		"note: Fix the request file parse error before running.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
}

func TestFromReportPreparesDefaultsAndCopiesData(t *testing.T) {
	reportSource := []byte("GET /users\n")
	itemSource := []byte("payload")
	frames := []diag.StackFrame{{Name: "call", Pos: diag.Pos{Line: 7}}}
	rep := diag.Report{
		Path:   "sample.http",
		Source: reportSource,
		Items: []diag.Diagnostic{{
			Message:  "failed",
			Severity: diag.Severity("bad"),
			Span:     diag.Span{Start: diag.Pos{Line: 2}},
			Labels: []diag.Label{
				{},
				{Message: "near value", Span: diag.Span{Start: diag.Pos{Line: 2}}},
			},
			Source: itemSource,
			Notes: []diag.Note{{
				Kind:    diag.NoteKind("bad"),
				Message: "  retry with a valid request  ",
			}},
			Frames: frames,
		}},
	}

	err := diag.FromReport(rep, nil)
	reportSource[0] = 'P'
	itemSource[0] = 'x'
	frames[0].Name = "changed"

	got := diag.ReportOf(err)
	if len(got.Items) != 1 {
		t.Fatalf("ReportOf() item count = %d, want 1", len(got.Items))
	}
	if string(got.Source) != "GET /users\n" {
		t.Fatalf("ReportOf().Source = %q, want original source", got.Source)
	}
	item := got.Items[0]
	if item.Class != diag.ClassUnknown {
		t.Fatalf("item class = %q, want %q", item.Class, diag.ClassUnknown)
	}
	if item.Severity != diag.SeverityError {
		t.Fatalf("item severity = %q, want %q", item.Severity, diag.SeverityError)
	}
	if item.Span.Start.Path != "sample.http" || item.Span.Start.Col != 1 {
		t.Fatalf("item start = %+v, want inherited path and column 1", item.Span.Start)
	}
	if item.Span.End.Path != "sample.http" {
		t.Fatalf("item end path = %q, want sample.http", item.Span.End.Path)
	}
	if len(item.Labels) != 1 {
		t.Fatalf("label count = %d, want 1", len(item.Labels))
	}
	if item.Labels[0].Span.Start.Path != "sample.http" || item.Labels[0].Span.Start.Col != 0 {
		t.Fatalf(
			"label start = %+v, want inherited path without column default",
			item.Labels[0].Span.Start,
		)
	}
	if string(item.Source) != "payload" {
		t.Fatalf("item source = %q, want original item source", item.Source)
	}
	if len(item.Notes) != 1 ||
		item.Notes[0].Kind != diag.NoteInfo ||
		item.Notes[0].Message != "  retry with a valid request  " {
		t.Fatalf("item notes = %#v, want preserved info note", item.Notes)
	}
	if len(item.Frames) != 1 || item.Frames[0].Name != "call" {
		t.Fatalf("item frames = %#v, want copied original frame", item.Frames)
	}
}

func TestFromReportPreparesNotesAndChain(t *testing.T) {
	err := diag.FromReport(diag.Report{
		Items: []diag.Diagnostic{{
			Class:   diag.ClassScript,
			Message: "failed",
			Notes: []diag.Note{
				{
					Kind:    diag.NoteKind("bad"),
					Message: " duplicate ",
					Span:    diag.Span{Start: diag.Pos{Line: 3}},
				},
				{
					Kind:    diag.NoteInfo,
					Message: "duplicate",
					Span:    diag.Span{Start: diag.Pos{Line: 3}},
				},
				{
					Kind:    diag.NoteHelp,
					Message: "duplicate",
					Span:    diag.Span{Start: diag.Pos{Line: 3}},
				},
				{Kind: diag.NoteInfo},
			},
			Chain: []diag.ChainEntry{
				{Kind: diag.ChainKind("bad"), Message: " root "},
				{Kind: diag.ChainCause, Class: diag.ClassUnknown, Message: "root"},
				{Children: []diag.ChainEntry{{Kind: diag.ChainKind("bad"), Message: " child "}}},
			},
		}},
	}, nil)

	got := diag.ReportOf(err)
	item := got.Items[0]
	if len(item.Notes) != 3 {
		t.Fatalf("note count = %d, want 3: %#v", len(item.Notes), item.Notes)
	}
	if item.Notes[0].Kind != diag.NoteInfo || item.Notes[0].Message != " duplicate " {
		t.Fatalf("first note = %#v, want preserved info duplicate", item.Notes[0])
	}
	if item.Notes[1].Kind != diag.NoteInfo || item.Notes[1].Message != "duplicate" {
		t.Fatalf("second note = %#v, want info duplicate", item.Notes[1])
	}
	if item.Notes[2].Kind != diag.NoteHelp || item.Notes[2].Message != "duplicate" {
		t.Fatalf("third note = %#v, want help duplicate", item.Notes[2])
	}
	if len(item.Chain) != 3 {
		t.Fatalf("chain count = %d, want 3: %#v", len(item.Chain), item.Chain)
	}
	if item.Chain[0].Kind != diag.ChainCause ||
		item.Chain[0].Class != diag.ClassUnknown ||
		item.Chain[0].Message != " root " {
		t.Fatalf("first chain entry = %#v, want preserved root cause", item.Chain[0])
	}
	if item.Chain[1].Kind != diag.ChainCause || item.Chain[1].Message != "root" {
		t.Fatalf("second chain entry = %#v, want exact root cause", item.Chain[1])
	}
	if item.Chain[2].Kind != diag.ChainCause || item.Chain[2].Message != " child " {
		t.Fatalf("third chain entry = %#v, want flattened preserved child cause", item.Chain[2])
	}
}

func TestClassesReturnsWrappedAndJoinedClasses(t *testing.T) {
	timeout := diag.New(diag.ClassTimeout, "timed out")
	auth := diag.WrapAs(diag.ClassAuth, timeout, "resolve auth")
	joined := diag.Join(
		diag.ClassProtocol,
		"resolve request",
		errors.New("plain"),
		auth,
		diag.New(diag.ClassAuth, "again"),
	)

	got := diag.Classes(joined)
	want := []diag.Class{diag.ClassProtocol, diag.ClassAuth, diag.ClassTimeout}
	if len(got) != len(want) {
		t.Fatalf("Classes(error) = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Classes(error) = %#v, want %#v", got, want)
		}
	}
}

func TestClassOfStopsAfterFirstClass(t *testing.T) {
	err := diag.Join(
		diag.ClassProtocol,
		"resolve request",
		diag.New(diag.ClassAuth, "token rejected"),
		panicUnwrapper{},
	)

	if got := diag.ClassOf(err); got != diag.ClassProtocol {
		t.Fatalf("ClassOf(error) = %q, want %q", got, diag.ClassProtocol)
	}
}

func TestHasClassStopsAfterMatch(t *testing.T) {
	err := diag.Join(
		diag.ClassProtocol,
		"resolve request",
		diag.New(diag.ClassAuth, "token rejected"),
		panicUnwrapper{},
	)

	if !diag.HasClass(err, diag.ClassProtocol) {
		t.Fatalf("HasClass(error, %q) = false, want true", diag.ClassProtocol)
	}
}

type panicUnwrapper struct{}

func (panicUnwrapper) Error() string { return "unreachable" }

func (panicUnwrapper) Unwrap() error {
	panic("class traversal should stop before this error")
}

func TestReportOfPrefersNestedReportThroughWrapper(t *testing.T) {
	base := diag.FromReport(diag.Report{
		Path: "bad.http",
		Items: []diag.Diagnostic{{
			Class:    diag.ClassParse,
			Severity: diag.SeverityError,
			Message:  "missing method",
			Span:     diag.Span{Start: diag.Pos{Line: 4, Col: 1}},
		}},
	}, nil)
	err := fmt.Errorf("run: %w", base)

	got := diag.Render(err)
	if strings.Contains(got, "run:") {
		t.Fatalf("Render() should use nested diagnostic report, got %q", got)
	}
	if !strings.Contains(got, "error[parse]: missing method") ||
		!strings.Contains(got, "--> bad.http:4:1") {
		t.Fatalf("Render() = %q", got)
	}
}

func TestRenderPlainErrorDoesNotDuplicateCause(t *testing.T) {
	got := diag.Render(errors.New("plain failure"))
	if got != "error[unknown]: plain failure" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestRenderLeafDiagnosticDoesNotDuplicateCause(t *testing.T) {
	err := diag.New(diag.ClassAuth, "token_url required")

	got := diag.Render(err)
	if got != "error[auth]: token_url required" {
		t.Fatalf("Render() = %q", got)
	}

	rep := diag.ReportOf(err)
	if len(rep.Items) != 1 {
		t.Fatalf("report items = %d, want 1", len(rep.Items))
	}
	if len(rep.Items[0].Chain) != 0 {
		t.Fatalf("leaf diagnostic chain = %#v, want empty", rep.Items[0].Chain)
	}
}

func TestRenderJoinedErrorsAsBranches(t *testing.T) {
	err := diag.Join(
		diag.ClassConfig,
		"load config",
		diag.New(diag.ClassAuth, "token rejected"),
		diag.New(diag.ClassTimeout, "command timed out"),
	)

	got := diag.Render(err)
	for _, want := range []string{
		"error[config]: load config",
		"├─> token rejected",
		"╰─> command timed out",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "\nerror[timeout]") {
		t.Fatalf("Render() should keep joined errors under one diagnostic: %q", got)
	}
	assertChainLines(t, err, []string{
		"load config",
		"├─> token rejected",
		"╰─> command timed out",
	})
}

func TestRenderWrappedRTSStack(t *testing.T) {
	err := diag.WrapAs(
		diag.ClassScript,
		&rts.StackError{
			Err: &rts.RuntimeError{
				Pos: rts.Pos{Path: "hook.rts", Line: 3, Col: 7},
				Msg: "boom",
			},
			Frames: []rts.Frame{{
				Kind: rts.FrameFn,
				Pos:  rts.Pos{Path: "hook.rts", Line: 2, Col: 1},
				Name: "sign",
			}},
		},
		"pre-request rts script",
	)

	got := diag.Render(err)
	for _, want := range []string{
		"error[script]: boom",
		"--> hook.rts:3:7",
		"pre-request rts script",
		"Stack:",
		"at hook.rts:2:1 in sign",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
	assertChainLines(t, err, []string{"pre-request rts script"})
}

func TestRenderHTTPTransportFailureUsesCauseClass(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "api.local"}
	err := diag.Wrap(
		&url.Error{Op: "Get", URL: "https://api.local", Err: dnsErr},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)

	if got := diag.ClassOf(err); got != diag.ClassNetwork {
		t.Fatalf("ClassOf(error) = %q, want %q", got, diag.ClassNetwork)
	}

	got := diag.Render(err)
	for _, want := range []string{
		"error[network]: request failed",
		"perform request",
		"╰─> Get \"https://api.local\"",
		"    ╰─> lookup api.local: no such host",
		"help: No response payload was received.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
	assertChainLines(t, err, []string{
		"perform request",
		`╰─> Get "https://api.local"`,
		"    ╰─> lookup api.local: no such host",
	})
}

func TestRenderURLWrappedRouteFailureDoesNotBecomeNetwork(t *testing.T) {
	err := diag.Wrap(
		&url.Error{
			Op:  "Get",
			URL: "http://api.default.svc.cluster.local/v1/health",
			Err: diag.WrapAs(
				diag.ClassRoute,
				errors.New(`context "kind-dev" does not exist`),
				"build kube client config",
			),
		},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)

	if got := diag.ClassOf(err); got != diag.ClassRoute {
		t.Fatalf("ClassOf(error) = %q, want %q", got, diag.ClassRoute)
	}

	got := diag.Render(err)
	for _, want := range []string{
		"error[route]: build kube client config",
		"perform request",
		`╰─> Get "http://api.default.svc.cluster.local/v1/health"`,
		`    ╰─> context "kind-dev" does not exist`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Render() missing %q in %q", want, got)
		}
	}
	for _, bad := range []string{
		"error[network]",
		"help: No response payload was received.",
	} {
		if strings.Contains(got, bad) {
			t.Fatalf("Render() should not contain %q in %q", bad, got)
		}
	}
	assertChainLines(t, err, []string{
		"perform request",
		`╰─> Get "http://api.default.svc.cluster.local/v1/health"`,
		`    ╰─> context "kind-dev" does not exist`,
	})
}

func TestWrapPreservesGoErrorChain(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "api.local"}
	err := diag.Wrap(dnsErr, "perform request")

	var got *net.DNSError
	if !errors.As(err, &got) || got != dnsErr {
		t.Fatalf("errors.As did not preserve wrapped DNS error")
	}
}

func assertChainLines(t *testing.T, err error, want []string) {
	t.Helper()

	var got []string
	for _, line := range diag.Lines(diag.ReportOf(err)) {
		if line.Kind == diag.LineChain {
			got = append(got, line.Text)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("chain lines = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chain lines = %#v, want %#v", got, want)
		}
	}
}
