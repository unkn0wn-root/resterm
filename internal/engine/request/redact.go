package request

import (
	"slices"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/redact"
)

// Results are redacted at the engine boundary because errors and test output
// can be rendered directly by the UI, workflows, and history. The complete
// secret set is returned for callers that render additional text. Wrapped
// errors retain their original cause for errors.Is and errors.As.
func redactResult(res engine.RequestResult) engine.RequestResult {
	sec := redact.New(res.RuntimeSecrets...)
	res.Err = diag.Redact(res.Err, sec.Text)
	res.ScriptErr = diag.Redact(res.ScriptErr, sec.Text)
	res.Tests = slices.Clone(res.Tests)
	for i := range res.Tests {
		res.Tests[i].Name = sec.Text(res.Tests[i].Name)
		res.Tests[i].Message = sec.Text(res.Tests[i].Message)
	}
	if res.Stream != nil {
		st := *res.Stream
		st.Err = diag.Redact(st.Err, sec.Text)
		res.Stream = &st
	}
	res.Warnings = slices.Clone(res.Warnings)
	for i := range res.Warnings {
		res.Warnings[i] = sec.Text(res.Warnings[i])
	}
	res.SkipReason = sec.Text(res.SkipReason)
	return res
}
