package diag_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func maskSecrets(secrets ...string) func(string) string {
	return func(s string) string {
		for _, secret := range secrets {
			s = strings.ReplaceAll(s, secret, "•••")
		}
		return s
	}
}

func reportAt(source, want string) diag.Report {
	at := strings.Index(source, want)
	return diag.Report{
		Path:   "review.http",
		Source: []byte(source),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassProtocol,
			Severity: diag.SeverityError,
			Message:  "undefined variable: missing",
			Span: diag.Span{Start: diag.Pos{
				Line: strings.Count(source[:at], "\n") + 1,
				Col:  at - strings.LastIndex(source[:at], "\n"),
			}},
		}},
	}
}

func assertLocationKept(t *testing.T, rep diag.Report, rendered string) {
	t.Helper()

	loc := fmt.Sprintf("--> review.http:%d:%d", rep.Items[0].Span.Start.Line, rep.Items[0].Span.Start.Col)
	if !strings.Contains(rendered, loc) {
		t.Errorf("report lost the file location %q:\n%s", loc, rendered)
	}
}

func TestRedactKeepsTheCaretUnderTheSpan(t *testing.T) {
	const secret = "very-long-password-12345"
	source := "POST https://example.invalid\n\n" +
		`{"password":"` + secret + `","token":"{{missing}}"}` + "\n"

	rep := reportAt(source, "{{missing}}")
	redacted := rep.Redact(maskSecrets(secret))

	rendered := diag.RenderReport(redacted)
	if strings.Contains(rendered, secret) {
		t.Fatalf("report kept the secret:\n%s", rendered)
	}
	assertLocationKept(t, rep, rendered)
	assertCaretUnder(t, redacted, "{{missing}}")
}

func TestRedactMapsTheExcerptAcrossLineEndings(t *testing.T) {
	for _, file := range []string{"\n", "\r\n"} {
		for _, secret := range []string{"\n", "\r\n"} {
			for _, sameLine := range []bool{true, false} {
				gap := file
				if sameLine {
					gap = " "
				}
				name := fmt.Sprintf("file=%q/secret=%q/same-line=%t", file, secret, sameLine)
				t.Run(name, func(t *testing.T) {
					source := "POST https://example.invalid" + file + file +
						"private-key-line-1" + file + "private-key-line-2" + gap +
						"other-long-secret {{missing}}" + file + "trailer" + file

					rep := reportAt(source, "{{missing}}")
					redacted := rep.Redact(maskSecrets(
						"private-key-line-1"+secret+"private-key-line-2",
						"other-long-secret",
					))

					rendered := diag.RenderReport(redacted)
					if strings.Contains(rendered, "other-long-secret") {
						t.Fatalf("report kept the secret:\n%s", rendered)
					}
					assertLocationKept(t, rep, rendered)
					assertCaretUnder(t, redacted, "{{missing}}")
				})
			}
		}
	}
}

func TestRedactTwiceKeepsTheCaretUnderTheSpan(t *testing.T) {
	source := "POST https://example.invalid\n\n" +
		"alpha-secret-1\nalpha-secret-2 beta-secret {{missing}}\ntrailer\n"

	rep := reportAt(source, "{{missing}}")
	redacted := rep.
		Redact(maskSecrets("alpha-secret-1\nalpha-secret-2")).
		Redact(maskSecrets("beta-secret"))

	rendered := diag.RenderReport(redacted)
	if strings.Contains(rendered, "secret") {
		t.Fatalf("report kept a secret:\n%s", rendered)
	}
	assertLocationKept(t, rep, rendered)
	assertCaretUnder(t, redacted, "{{missing}}")
}

func FuzzRedactKeepsTheCaretUnderTheSpan(f *testing.F) {
	const want = "{{missing}}"
	f.Add("POST /x\n\nalpha beta "+want+"\n", "alpha")
	f.Add("alpha\r\nbeta "+want+"\r\ntrailer\r\n", "alpha\r\nbeta")
	f.Add("alpha\r\nbeta "+want+"\r\ntrailer\r\n", "alpha\nbeta")
	f.Add("\tname: \"名前\" "+want, "名前")
	f.Add("\x9b\t1"+want, "0")
	f.Fuzz(func(t *testing.T, source, secret string) {
		mask := maskSecrets(secret)
		if secret == "" || strings.Count(source, want) != 1 || strings.Count(mask(source), want) != 1 {
			t.Skip()
		}
		assertCaretUnder(t, reportAt(source, want).Redact(mask), want)
	})
}

func TestRedactMasksEachJoinedExcerptInItsOwnFile(t *testing.T) {
	a := reportAt("GET /a\nalpha-secret {{missing}}\n", "{{missing}}")
	b := reportAt("let x = 1\nlet y = 2\nbeta-secret {{other}}\n", "{{other}}")
	b.Path = "b.rts"
	joined := errors.Join(diag.FromReport(a, nil), diag.FromReport(b, nil))

	rep := diag.ReportOf(joined).Redact(maskSecrets("alpha-secret", "beta-secret"))
	if rendered := diag.RenderReport(rep); strings.Contains(rendered, "secret") {
		t.Fatalf("report kept a secret:\n%s", rendered)
	}
	first, second := rep, rep
	first.Items, second.Items = rep.Items[:1], rep.Items[1:]
	assertCaretUnder(t, first, "{{missing}}")
	assertCaretUnder(t, second, "{{other}}")
}

func TestRedactError(t *testing.T) {
	const secret = "s3cr3t-value"
	sourceOnly := func(sec string) error {
		return diag.FromReport(diag.Report{
			Path:   "secret.http",
			Source: []byte("GET http://example.test\n\n" + sec + " trailing\n"),
			Items: []diag.Diagnostic{{
				Class:    diag.ClassScript,
				Severity: diag.SeverityError,
				Message:  "undefined variable: missing",
				Span:     diag.Span{Start: diag.Pos{Line: 3, Col: 1}, End: diag.Pos{Line: 3, Col: 1}},
			}},
		}, errors.New("undefined variable: missing"))
	}
	for _, sec := range []string{
		secret,
		secret + "\tmore",
		secret + string(rune(0xad)) + "more",
		secret + string(rune(0x200b)) + "more",
		secret + string(rune(0xfeff)) + "more",
		secret + string(rune(0xe0001)) + "more",
	} {
		err := sourceOnly(sec)
		out := diag.Redact(err, maskSecrets(sec))
		if got := diag.Render(out); strings.Contains(got, secret) {
			t.Errorf("rendered diagnostic shows %q:\n%s", sec, got)
		}
		if src := diag.ReportOf(out).Source; strings.Contains(string(src), secret) {
			t.Errorf("report source keeps %q: %q", sec, src)
		}
		if !errors.Is(out, err) {
			t.Errorf("Redact(%v) does not unwrap to the error", err)
		}
	}

	cause := errors.New("dial " + secret)
	wrapped := fmt.Errorf("perform request: %w", cause)
	if out := diag.Redact(
		wrapped,
		maskSecrets(secret),
	); out.Error() != "perform request: dial •••" ||
		!errors.Is(out, cause) {
		t.Errorf("Redact(%v) = %v, want the text masked and the cause kept", wrapped, out)
	}
	if out := diag.Redact(wrapped, maskSecrets("other")); out != wrapped {
		t.Errorf("Redact without a secret = %#v, want the error itself", out)
	}
	if out := diag.Redact(nil, maskSecrets(secret)); out != nil {
		t.Errorf("Redact(nil) = %v", out)
	}
}
