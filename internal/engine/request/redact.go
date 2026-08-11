package request

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Results are redacted at the engine boundary because errors and test output
// can be rendered directly by the UI, workflows, and history. The complete
// secret set is returned for callers that render additional text. Wrapped
// errors retain their original cause for errors.Is and errors.As.
func (e *Engine) redactResult(
	res xrunResult,
	doc *restfile.Document,
	env vars.Environment,
) xrunResult {
	secs := e.secretValues(doc, res.Executed, env, res.RuntimeSecrets...)
	if len(secs) == 0 {
		return res
	}
	res.RuntimeSecrets = secs
	res.Err = redactErr(res.Err, secs)
	res.ScriptErr = redactErr(res.ScriptErr, secs)
	for i := range res.Tests {
		res.Tests[i].Name = redactSecretText(res.Tests[i].Name, secs)
		res.Tests[i].Message = redactSecretText(res.Tests[i].Message, secs)
	}
	return res
}

type redactedError struct {
	err  error
	text string
	rep  diag.Report
}

func (e *redactedError) Error() string           { return e.text }
func (e *redactedError) Unwrap() error           { return e.err }
func (e *redactedError) Diagnostic() diag.Report { return e.rep }

func redactErr(err error, secs []string) error {
	if err == nil {
		return nil
	}
	text := err.Error()
	if !containsSecret(text, secs) && !containsSecret(diag.Render(err), secs) {
		return err
	}
	mask := func(s string) string { return redactSecretText(s, secs) }
	return &redactedError{
		err:  err,
		text: mask(text),
		rep:  diag.ReportOf(err).Redact(mask),
	}
}

func containsSecret(s string, secs []string) bool {
	for _, sec := range secs {
		if sec != "" && strings.Contains(s, sec) {
			return true
		}
	}
	return false
}
