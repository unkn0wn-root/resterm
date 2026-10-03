package vars

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/vars/dynamic"
)

// InterpolateOptions sets limits for Interpolate.
type InterpolateOptions struct {
	// MaxLen limits the result length in bytes. Zero means no limit.
	MaxLen int
}

// Interpolate fills placeholders using look and dynamic helpers.
// Inserted values stay as written. Expressions, missing variables, and invalid
// placeholders return an error. It stops before the result passes MaxLen, and
// at the first error a later placeholder could not replace.
func Interpolate(text string, look Lookup, opt InterpolateOptions) (string, error) {
	if u := UnclosedPlaceholders(text, diag.Pos{}); len(u) > 0 {
		return "", errors.New(u[0].Message())
	}
	value := func(seg tplSeg) (string, error) {
		switch {
		case seg.name == "":
			return "", fmt.Errorf("placeholder %s has no name", seg.text)
		case seg.name[0] == '=':
			return "", fmt.Errorf("expression %s is not allowed", seg.text)
		}
		v, ok, err := look(seg.name)
		if err != nil || ok {
			return v, err
		}
		if seg.name[0] == '$' {
			v, err = dynamic.Resolve(seg.name)
			if !errors.Is(err, dynamic.ErrUnknown) {
				return v, err
			}
		}
		return "", fmt.Errorf("%w: %s", ErrUndefinedVariable, seg.name)
	}

	var b strings.Builder
	var firstErr error
	for _, seg := range CompileTemplate(text).segs {
		v := seg.text
		if seg.ph {
			var err error
			if v, err = value(seg); err != nil {
				firstErr = PreferStructural(firstErr, err)
				if !errors.Is(firstErr, ErrUndefinedVariable) {
					return "", firstErr
				}
				continue
			}
		}
		if firstErr != nil {
			continue
		}
		if opt.MaxLen > 0 && b.Len()+len(v) > opt.MaxLen {
			return "", fmt.Errorf("result is longer than %d bytes", opt.MaxLen)
		}
		b.WriteString(v)
	}
	if firstErr != nil {
		return "", firstErr
	}
	return b.String(), nil
}
