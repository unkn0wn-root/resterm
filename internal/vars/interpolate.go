package vars

import (
	"errors"
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/vars/dynamic"
)

// Interpolate fills placeholders using look and dynamic helpers.
// Inserted values stay as written. Expressions, missing variables, and invalid
// placeholders return an error.
func Interpolate(text string, look Lookup) (string, error) {
	if u := UnclosedPlaceholders(text, diag.Pos{}); len(u) > 0 {
		return "", errors.New(u[0].Message())
	}
	var firstErr error
	out := CompileTemplate(text).replace(func(seg tplSeg, _ int) string {
		fail := func(err error) string {
			firstErr = PreferStructural(firstErr, err)
			return seg.text
		}
		switch {
		case seg.name == "":
			return fail(fmt.Errorf("placeholder %s has no name", seg.text))
		case seg.name[0] == '=':
			return fail(fmt.Errorf("expression %s is not allowed", seg.text))
		}
		v, ok, err := look(seg.name)
		if err != nil {
			return fail(err)
		}
		if ok {
			return v
		}
		if seg.name[0] == '$' {
			v, err = dynamic.Resolve(seg.name)
			if err == nil {
				return v
			}
			if !errors.Is(err, dynamic.ErrUnknown) {
				return fail(err)
			}
		}
		return fail(fmt.Errorf("%w: %s", ErrUndefinedVariable, seg.name))
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}
