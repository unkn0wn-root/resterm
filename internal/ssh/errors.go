package ssh

import "errors"

func joinCloseErr(baseErr error, cleanupErr error) error {
	if cleanupErr == nil {
		return baseErr
	}
	if baseErr == nil {
		return cleanupErr
	}
	return errors.Join(baseErr, cleanupErr)
}
