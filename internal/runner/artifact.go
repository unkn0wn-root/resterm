package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

func (r *Report) writeArtifacts(dir string) error {
	if r == nil {
		return nil
	}
	if dir == "" {
		return nil
	}
	streamsDir := filepath.Join(dir, "streams")
	tracesDir := filepath.Join(dir, "traces")
	for i := range r.Results {
		item := &r.Results[i]
		if path, err := writeStreamArtifact(
			streamsDir,
			i+1,
			0,
			resultName(*item),
			item.Stream,
			item.transcript,
		); err != nil {
			return err
		} else if item.Stream != nil {
			item.Stream.TranscriptPath = path
		}
		if path, err := writeTraceArtifact(
			tracesDir,
			i+1,
			0,
			resultName(*item),
			item.Trace,
		); err != nil {
			return err
		} else if item.Trace != nil {
			item.Trace.ArtifactPath = path
		}
		for j := range item.Steps {
			step := &item.Steps[j]
			if path, err := writeStreamArtifact(
				streamsDir,
				i+1,
				j+1,
				stepName(*step),
				step.Stream,
				step.transcript,
			); err != nil {
				return err
			} else if step.Stream != nil {
				step.Stream.TranscriptPath = path
			}
			if path, err := writeTraceArtifact(
				tracesDir,
				i+1,
				j+1,
				stepName(*step),
				step.Trace,
			); err != nil {
				return err
			} else if step.Trace != nil {
				step.Trace.ArtifactPath = path
			}
		}
	}
	return nil
}

func writeStreamArtifact(
	base string,
	resultIndex int,
	stepIndex int,
	name string,
	stream *StreamInfo,
	transcript []byte,
) (string, error) {
	if stream == nil || len(transcript) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	file := fmt.Sprintf("result-%03d", resultIndex)
	if stepIndex > 0 {
		file += fmt.Sprintf("-step-%03d", stepIndex)
	}
	if slug := streamArtifactSlug(name); slug != "" {
		file += "-" + slug
	}
	if kind := str.LowerTrim(stream.Kind); kind != "" {
		file += "-" + kind
	}
	path := filepath.Join(base, file+".json")
	if err := os.WriteFile(path, transcript, 0o644); err != nil {
		return "", fmt.Errorf("write stream artifact: %w", err)
	}
	return path, nil
}

func writeTraceArtifact(
	base string,
	resultIndex int,
	stepIndex int,
	name string,
	trace *TraceInfo,
) (string, error) {
	if trace == nil || trace.Summary == nil {
		return "", nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	file := fmt.Sprintf("result-%03d", resultIndex)
	if stepIndex > 0 {
		file += fmt.Sprintf("-step-%03d", stepIndex)
	}
	if slug := streamArtifactSlug(name); slug != "" {
		file += "-" + slug
	}
	path := filepath.Join(base, file+"-trace.json")
	data, err := json.MarshalIndent(trace.Summary, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal trace artifact: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write trace artifact: %w", err)
	}
	return path, nil
}

func streamArtifactSlug(name string) string {
	name = str.LowerTrim(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if lastDash || b.Len() == 0 {
			continue
		}
		b.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(b.String(), "-")
}

func resultName(item Result) string {
	name := item.Name
	if name != "" {
		return name
	}
	target := item.Target
	if target == "" {
		return "<unnamed>"
	}
	if len(target) > 80 {
		return target[:77] + "..."
	}
	return target
}

func stepName(step StepResult) string {
	name := step.Name
	if name != "" {
		return name
	}
	if env := step.Environment; env != "" {
		return env
	}
	target := step.Target
	if target != "" {
		return target
	}
	return "<step>"
}
