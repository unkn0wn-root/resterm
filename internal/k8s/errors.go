package k8s

import "errors"

func joinCleanupErr(err, cleanupErr error) error {
	switch {
	case err == nil:
		return cleanupErr
	case cleanupErr == nil:
		return err
	default:
		return errors.Join(err, cleanupErr)
	}
}
