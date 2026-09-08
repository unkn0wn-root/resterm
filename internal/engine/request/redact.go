package request

import (
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
	env vars.ResolvedEnv,
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

// The mask reports whether it replaced anything, so the decision to redact and
// the redaction itself read the same fields. Testing rendered text instead would
// miss a secret that display escaping has rewritten.
func redactErr(err error, secs []string) error {
	if err == nil {
		return nil
	}
	found := false
	mask := func(s string) string {
		out := redactSecretText(s, secs)
		found = found || out != s
		return out
	}
	text := mask(err.Error())
	rep := diag.ReportOf(err).Redact(mask)
	if !found {
		return err
	}
	return &redactedError{err: err, text: text, rep: rep}
}
