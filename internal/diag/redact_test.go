package diag_test

import (
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
