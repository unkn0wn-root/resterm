package runcheck

import "fmt"

type Names struct {
	Profile  string
	Compare  string
	Workflow string
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
