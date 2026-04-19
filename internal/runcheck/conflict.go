package runcheck

import (
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Names struct {
	Profile  string
	Compare  string
	Workflow string
}

// ValidateConcreteEnvironment rejects reserved pseudo-environment names.
func ValidateConcreteEnvironment(v, site string) error {
	if !vars.IsReservedEnvironment(v) {
		return nil
	}
	return fmt.Errorf(
		"%s %q is reserved for shared defaults. Choose a concrete environment",
		site,
		v,
	)
}

func ValidateProfileCompare(profile, compare bool, n Names) error {
	if !profile || !compare {
		return nil
	}
	return fmt.Errorf("%s cannot be combined with %s", n.Profile, n.Compare)
}

func ValidateWorkflowMode(workflow, profile, compare bool, n Names) error {
	if !workflow || (!profile && !compare) {
		return nil
	}
	return fmt.Errorf("%s cannot be combined with %s or %s", n.Workflow, n.Compare, n.Profile)
}
