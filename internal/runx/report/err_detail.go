package runfmt

import "github.com/unkn0wn-root/resterm/internal/diag"

// ErrorDetailFromError converts err into the report-layer error detail form.
func ErrorDetailFromError(err error) *ErrorDetail {
	if err == nil {
		return nil
	}
	rep := diag.ReportOf(err)
	if len(rep.Items) == 0 {
		return nil
	}
	item := rep.Items[0]
	return &ErrorDetail{
		Code:      string(rep.Class()),
		Component: string(item.Component),
		Severity:  string(item.Severity),
		Message:   rep.Summary(),
		Rendered:  diag.RenderReport(rep),
		Chain:     failureChain(item.Chain),
		Frames:    failureFrames(item.Frames),
	}
}

func failureChain(src []diag.ChainEntry) []FailureChain {
	if len(src) == 0 {
		return nil
	}
	out := make([]FailureChain, 0, len(src))
	for _, entry := range src {
		out = append(out, FailureChain{
			Code:      string(entry.Class),
			Component: string(entry.Component),
			Kind:      string(entry.Kind),
			Message:   entry.Message,
			Children:  failureChain(entry.Children),
		})
	}
	return out
}

func failureFrames(src []diag.StackFrame) []FailureFrame {
	if len(src) == 0 {
		return nil
	}
	out := make([]FailureFrame, 0, len(src))
	for _, frame := range src {
		out = append(out, FailureFrame{
			Name: frame.Name,
			Pos:  failurePos(frame.Pos),
		})
	}
	return out
}

func failurePos(pos diag.Pos) FailurePos {
	return FailurePos{Path: pos.Path, Line: pos.Line, Col: pos.Col}
}

func errorDetailText(detail *ErrorDetail, fallback string) string {
	if detail == nil || detail.Rendered == "" {
		return fallback
	}
	return detail.Rendered
}

// AttachErrorDetail copies chain and frame metadata from detail onto failure.
func AttachErrorDetail(failure *Failure, detail *ErrorDetail) *Failure {
	if failure == nil || detail == nil {
		return failure
	}
	failure.Chain = CloneFailureChain(detail.Chain)
	failure.Frames = CloneFailureFrames(detail.Frames)
	return failure
}

// CloneFailureChain returns a deep copy of src.
func CloneFailureChain(src []FailureChain) []FailureChain {
	if len(src) == 0 {
		return nil
	}
	out := make([]FailureChain, 0, len(src))
	for _, entry := range src {
		entry.Children = CloneFailureChain(entry.Children)
		out = append(out, entry)
	}
	return out
}

// CloneFailureFrames returns a copy of src.
func CloneFailureFrames(src []FailureFrame) []FailureFrame {
	if len(src) == 0 {
		return nil
	}
	return append([]FailureFrame(nil), src...)
}
