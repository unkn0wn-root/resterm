package cli

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func ParseCompareTargets(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
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
		if vars.IsReservedEnvironment(field) {
			return nil, fmt.Errorf("environment %q is reserved for shared defaults", field)
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

func ValidateReservedEnvironment(value, flagName string) error {
	if vars.IsReservedEnvironment(value) {
		return fmt.Errorf(
			"%s %q is reserved for shared defaults; choose a concrete environment",
			flagName,
			value,
		)
	}
	return nil
}
