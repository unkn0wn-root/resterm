package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) setProfile(d parsedDirective) directiveOutcome {
	spec, err := parseProfileSpec(d.Args, d.lines.Start)
	b.reportStrict(d, err)
	if spec == nil {
		return directiveRejected
	}
	b.request.metadata.Profile = spec
	return directiveApplied
}

func (b *documentBuilder) lintProfile(req *restfile.Request) {
	if spec := req.Metadata.Profile; spec != nil && req.GRPC != nil {
		b.addError(spec.Line, "@profile is not supported for gRPC requests")
	}
}

// A bare value sets count. Combined values must use named options.
func parseProfileSpec(raw string, line int) (*restfile.ProfileSpec, error) {
	spec := &restfile.ProfileSpec{Count: restfile.DefaultProfileCount, Line: line}
	fields := directive.Fields(raw)
	if len(fields) > 0 && !strings.Contains(fields[0], "=") {
		if len(fields) > 1 {
			if _, err := strconv.Atoi(fields[0]); err == nil {
				return nil, fmt.Errorf("use count=%s to combine a count with other @profile options", fields[0])
			}
		} else {
			n, err := positiveInt("@profile count", fields[0])
			if err != nil {
				return nil, err
			}
			spec.Count = n
			return spec, nil
		}
	}

	opts, err := directive.ParseOptions(directive.Profile, raw)
	errs := []error{err}
	if opts.Has("count") {
		spec.Count, err = positiveInt("@profile count", opts.Pop("count"))
		errs = append(errs, err)
	}
	if opts.Has("warmup") {
		spec.Warmup, err = nonNegativeInt("@profile warmup", opts.Pop("warmup"))
		errs = append(errs, err)
	}
	if opts.Has("delay") {
		spec.Delay, err = nonNegativeDuration("@profile delay", opts.Pop("delay"))
		errs = append(errs, err)
	}
	errs = append(errs, opts.Leftover(directive.Profile))
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return spec, nil
}

func positiveInt(field, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", field, raw)
	}
	return n, nil
}

func nonNegativeInt(field, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", field, raw)
	}
	return n, nil
}
