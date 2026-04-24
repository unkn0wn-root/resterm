package cli

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/runx/check"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func ParseCompareTargets(raw string) ([]string, error) {
	raw = str.Trim(raw)
	if raw == "" {
		return nil, nil
	}

	repl := strings.NewReplacer(",", " ", ";", " ")
	fields := strings.Fields(repl.Replace(raw))
	if len(fields) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(fields))
	targets := make([]string, 0, len(fields))
	for _, field := range fields {
		if err := runcheck.ValidateConcreteEnvironment(field, "environment"); err != nil {
			return nil, err
		}
		key := strings.ToLower(field)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, field)
	}

	if len(targets) < 2 {
		return nil, fmt.Errorf("expected at least two environments, got %d", len(targets))
	}
	return targets, nil
}
