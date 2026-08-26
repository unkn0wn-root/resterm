package parser

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	defaultPollEvery        = time.Second
	defaultPollTimeout      = 30 * time.Second
	defaultRetryInitial     = 100 * time.Millisecond
	defaultRetryMax         = 2 * time.Second
	defaultRetryJitter      = 20
	maxRetryJitterPercent   = 100
	exponentialBackoffToken = "exponential"
)

type retryDraft struct {
	base        *restfile.RetrySpec
	when        *restfile.ResponsePredicate
	backoff     *restfile.ExponentialBackoffSpec
	backoffLine int
}

func (b *documentBuilder) setPoll(d parsedDirective) directiveOutcome {
	spec, err := parsePollSpec(d.Args, d.lines.Start)
	b.report(d.lines.Start, err)
	if spec == nil || fatalErr(err) {
		return directiveRejected
	}
	d.setExprCol(&spec.Until.Col, spec.Until.Expression)
	b.request.metadata.Poll = spec
	return directiveApplied
}

func (b *documentBuilder) setRetry(d parsedDirective) directiveOutcome {
	spec, err := parseRetrySpec(d.Args, d.lines.Start)
	b.report(d.lines.Start, err)
	if spec == nil || fatalErr(err) {
		return directiveRejected
	}
	b.retryDraft().base = spec
	return directiveApplied
}

func (b *documentBuilder) setRetryWhen(d parsedDirective) directiveOutcome {
	expr := cutAssign(d.Args)
	if expr == "" {
		return b.reject(d, "@retry-when expression missing")
	}
	pred := &restfile.ResponsePredicate{Expression: expr, Line: d.lines.Start}
	d.setExprCol(&pred.Col, pred.Expression)
	b.retryDraft().when = pred
	return directiveApplied
}

func (b *documentBuilder) setRetryBackoff(d parsedDirective) directiveOutcome {
	spec, err := parseRetryBackoff(d.Args)
	b.report(d.lines.Start, err)
	if spec == nil || fatalErr(err) {
		return directiveRejected
	}
	draft := b.retryDraft()
	draft.backoff = spec
	draft.backoffLine = d.lines.Start
	return directiveApplied
}

func (b *documentBuilder) retryDraft() *retryDraft {
	if b.request.retry == nil {
		b.request.retry = &retryDraft{}
	}
	return b.request.retry
}

// @retry-when and @retry-backoff may appear before @retry.
func (b *documentBuilder) finalizeRetry() {
	draft := b.request.retry
	if draft == nil {
		return
	}
	if draft.base == nil {
		if draft.when != nil {
			b.addError(draft.when.Line, "@retry-when requires @retry")
		}
		if draft.backoff != nil {
			b.addError(draft.backoffLine, "@retry-backoff requires @retry")
		}
		return
	}

	spec := *draft.base
	if draft.when != nil {
		spec.When = draft.when
	}
	if draft.backoff != nil {
		spec.Backoff = *draft.backoff
	}
	b.request.metadata.Retry = &spec
}

func (b *documentBuilder) lintRequestPolicies(req *restfile.Request) {
	line := 0
	switch {
	case req.Metadata.Poll != nil:
		line = req.Metadata.Poll.Line
	case req.Metadata.Retry != nil:
		line = req.Metadata.Retry.Line
	default:
		return
	}

	if req.Metadata.Profile != nil {
		b.addError(line, "@poll and @retry cannot be combined with @profile")
		return
	}
	if proto := req.RepeatUnsupported(); proto != "" {
		b.addError(line, "@poll and @retry are not supported for "+proto+" requests")
	}
}

func parsePollSpec(raw string, line int) (*restfile.PollSpec, error) {
	prefix, expr, ok := cutPollUntil(raw)
	if !ok || expr == "" {
		return nil, errors.New("@poll requires until=<expression> as its last option")
	}

	opts, err := directive.ParseOptions(directive.Poll, prefix)
	errs := []error{err}
	spec := &restfile.PollSpec{
		Every:   defaultPollEvery,
		Timeout: defaultPollTimeout,
		Until:   restfile.ResponsePredicate{Expression: expr, Line: line},
		Line:    line,
	}
	if value, found := opts.PopAny("every"); found {
		spec.Every, err = positiveDuration("@poll every", value)
		errs = append(errs, err)
	}
	if value, found := opts.PopAny("timeout"); found {
		spec.Timeout, err = positiveDuration("@poll timeout", value)
		errs = append(errs, err)
	}
	errs = append(errs, opts.Leftover(directive.Poll))
	err = errors.Join(errs...)
	if fatalErr(err) {
		return nil, err
	}
	return spec, err
}

// cutPollUntil ignores until= inside RTS strings and groups. The expression
// uses the rest of the directive, so until= must come last. Check each possible
// match separately because lowercasing the whole string can change byte offsets.
func cutPollUntil(raw string) (prefix, expr string, ok bool) {
	const marker = "until="
	masked := rts.Mask(raw)
	for at := 0; at+len(marker) <= len(masked); at++ {
		if !strings.EqualFold(masked[at:at+len(marker)], marker) {
			continue
		}
		prev, _ := utf8.DecodeLastRuneInString(masked[:at])
		if at == 0 || unicode.IsSpace(prev) {
			return strings.TrimSpace(raw[:at]), strings.TrimSpace(raw[at+len(marker):]), true
		}
	}
	return "", "", false
}

func parseRetrySpec(raw string, line int) (*restfile.RetrySpec, error) {
	opts, err := directive.ParseOptions(directive.Retry, raw)
	errs := []error{err}
	count := 0
	if countRaw, found := opts.PopAny("count"); !found {
		errs = append(errs, errors.New("@retry count is required"))
	} else {
		n, countErr := directive.ParseNonNegativeInt(countRaw)
		switch {
		case countErr != nil:
			countErr = fmt.Errorf("invalid @retry count %q: %w", countRaw, countErr)
		case n == 0:
			countErr = errors.New("@retry count must be greater than zero")
		}
		count, errs = n, append(errs, countErr)
	}
	errs = append(errs, opts.Leftover(directive.Retry))
	err = errors.Join(errs...)
	if fatalErr(err) {
		return nil, err
	}
	return &restfile.RetrySpec{
		Count: count,
		Backoff: restfile.ExponentialBackoffSpec{
			Initial:       defaultRetryInitial,
			Max:           defaultRetryMax,
			JitterPercent: defaultRetryJitter,
		},
		Line: line,
	}, err
}

func parseRetryBackoff(raw string) (*restfile.ExponentialBackoffSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("@retry-backoff requires exponential(initial, max)")
	}
	end := strings.IndexByte(raw, ')')
	if end < 0 {
		return nil, errors.New("@retry-backoff exponential is missing a closing parenthesis")
	}
	initial, maximum, err := parseExponentialCall(strings.TrimSpace(raw[:end+1]))
	if err != nil {
		return nil, err
	}

	opts, optionErr := directive.ParseOptions(directive.RetryBackoff, raw[end+1:])
	errs := []error{optionErr}
	jitter := float64(defaultRetryJitter)
	if rawJitter, found := opts.PopAny("jitter"); found {
		jitter, err = parseJitterPercent(rawJitter)
		errs = append(errs, err)
	}
	errs = append(errs, opts.Leftover(directive.RetryBackoff))
	if maximum < initial {
		errs = append(errs, errors.New("@retry-backoff maximum must be at least the initial delay"))
	}
	err = errors.Join(errs...)
	if fatalErr(err) {
		return nil, err
	}
	return &restfile.ExponentialBackoffSpec{
		Initial:       initial,
		Max:           maximum,
		JitterPercent: jitter,
	}, err
}

func parseExponentialCall(raw string) (time.Duration, time.Duration, error) {
	name, args, ok := strings.Cut(raw, "(")
	if !ok || !strings.EqualFold(strings.TrimSpace(name), exponentialBackoffToken) || !strings.HasSuffix(args, ")") {
		return 0, 0, fmt.Errorf("@retry-backoff must start with exponential(initial, max), got %q", raw)
	}
	parts := strings.Split(strings.TrimSuffix(args, ")"), ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("@retry-backoff exponential requires initial and maximum delays")
	}
	initial, err := positiveDuration("@retry-backoff initial delay", parts[0])
	if err != nil {
		return 0, 0, err
	}
	maximum, err := positiveDuration("@retry-backoff maximum delay", parts[1])
	if err != nil {
		return 0, 0, err
	}
	return initial, maximum, nil
}

func parseJitterPercent(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if !strings.HasSuffix(value, "%") {
		return 0, fmt.Errorf("@retry-backoff jitter must be a percentage, got %q", raw)
	}
	percent, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 || percent > maxRetryJitterPercent {
		return 0, fmt.Errorf("@retry-backoff jitter must be between 0%% and 100%%, got %q", raw)
	}
	return percent, nil
}

func positiveDuration(field, raw string) (time.Duration, error) {
	value, ok := duration.Parse(strings.TrimSpace(raw))
	if !ok || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", field, strings.TrimSpace(raw))
	}
	return value, nil
}
