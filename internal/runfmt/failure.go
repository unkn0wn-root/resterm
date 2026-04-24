package runfmt

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/runfail"
)

const ReportSchemaVersion = "1"

func (rep Report) ExitCode(mode runfail.ExitMode) int {
	failures := rep.Failures()
	return runfail.ExitCode(failures, rep.Failed > 0 || len(failures) > 0, mode)
}

func (rep Report) FailureCodes() []string {
	failures := rep.Failures()
	if len(failures) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(failures))
	out := make([]string, 0, len(failures))
	for _, failure := range failures {
		code := strings.TrimSpace(string(failure.Code))
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

func (rep Report) Failures() []runfail.Failure {
	out := make([]runfail.Failure, 0)
	for _, res := range rep.Results {
		if failure := toRunFailure(res.Failure); failure.Code != "" {
			out = append(out, failure)
		}
		for _, step := range res.Steps {
			if failure := toRunFailure(step.Failure); failure.Code != "" {
				out = append(out, failure)
			}
		}
		if res.Profile != nil {
			for _, failure := range res.Profile.Failures {
				if got := toRunFailure(failure.Failure); got.Code != "" {
					out = append(out, got)
				}
			}
		}
	}
	return out
}

func FromFailure(f runfail.Failure) *Failure {
	if f.Code == "" {
		return nil
	}
	return &Failure{
		Code:     f.Code,
		Category: f.Category,
		ExitCode: f.ExitCode,
		Message:  f.Message,
		Source:   f.Source,
	}
}

func toRunFailure(f *Failure) runfail.Failure {
	if f == nil {
		return runfail.Failure{}
	}
	return runfail.New(f.Code, f.Message, f.Source)
}

func schemaVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return ReportSchemaVersion
	}
	return strings.TrimSpace(v)
}
