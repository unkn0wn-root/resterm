package runfmt

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/runclass"
)

const ReportSchemaVersion = "1"

func (rep Report) ExitCode(mode runclass.ExitCodeMode) int {
	failures := rep.Failures()
	return runclass.ReportExitCode(failures, rep.Failed > 0 || len(failures) > 0, mode)
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

func (rep Report) Failures() []runclass.Failure {
	out := make([]runclass.Failure, 0)
	for _, res := range rep.Results {
		if failure := failureToClass(res.Failure); failure.Code != "" {
			out = append(out, failure)
		}
		for _, step := range res.Steps {
			if failure := failureToClass(step.Failure); failure.Code != "" {
				out = append(out, failure)
			}
		}
		if res.Profile != nil {
			for _, failure := range res.Profile.Failures {
				if got := failureToClass(failure.Failure); got.Code != "" {
					out = append(out, got)
				}
			}
		}
	}
	return out
}

func ClassFailure(f runclass.Failure) *Failure {
	if f.Code == "" {
		return nil
	}
	return &Failure{
		Code:     string(f.Code),
		Category: string(f.Category),
		ExitCode: f.ExitCode,
		Message:  f.Message,
		Source:   f.Source,
	}
}

func failureToClass(f *Failure) runclass.Failure {
	if f == nil {
		return runclass.Failure{}
	}
	return runclass.NewFailure(runclass.FailureCode(f.Code), f.Message, f.Source)
}

func schemaVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return ReportSchemaVersion
	}
	return strings.TrimSpace(v)
}
