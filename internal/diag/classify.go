package diag

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type errUnwrapper interface {
	Unwrap() error
}

type errsUnwrapper interface {
	Unwrap() []error
}

func classify(err error) Class {
	if err == nil {
		return ClassUnknown
	}
	if e, ok := err.(*diagnosticError); ok {
		return classFromError(e)
	}
	if rep, ok := err.(Reporter); ok {
		if class := rep.Diagnostic().Class(); class != ClassUnknown {
			return class
		}
	}

	if errors.Is(err, context.Canceled) {
		return ClassCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return ClassTimeout
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return ClassTimeout
	}
	if st, ok := status.FromError(err); ok {
		if class := grpcStatusClass(st.Code()); class != ClassUnknown {
			return class
		}
	}

	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		if class := classify(urlErr.Err); class != ClassUnknown {
			return class
		}
		return ClassNetwork
	}

	if isTLSError(err) {
		return ClassTLS
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return ClassNetwork
	}

	if _, ok := errors.AsType[*net.OpError](err); ok {
		return ClassNetwork
	}

	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission),
		errors.Is(err, fs.ErrExist),
		errors.Is(err, fs.ErrClosed):
		return ClassFilesystem
	}

	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return ClassFilesystem
	}

	if wrapped, ok := err.(errsUnwrapper); ok {
		return dominantClass(wrapped.Unwrap())
	}

	if wrapped, ok := err.(errUnwrapper); ok {
		return classify(wrapped.Unwrap())
	}

	return ClassUnknown
}

func dominantClass(errs []error) Class {
	var out Class
	for _, err := range errs {
		class := classify(err)
		if class == ClassUnknown {
			continue
		}
		if out == "" || classRank(class) < classRank(out) {
			out = class
		}
	}
	if out == "" {
		return ClassUnknown
	}
	return out
}

func classRank(class Class) int {
	switch class {
	case ClassCanceled:
		return 0
	case ClassTimeout:
		return 10
	case ClassNetwork:
		return 20
	case ClassTLS:
		return 30
	case ClassAuth:
		return 40
	case ClassScript:
		return 50
	case ClassFilesystem:
		return 60
	case ClassProtocol:
		return 70
	case ClassRoute:
		return 80
	case ClassConfig, ClassParse, ClassHistory, ClassUI, ClassInternal:
		return 90
	default:
		return 100
	}
}

func grpcStatusClass(code codes.Code) Class {
	switch code {
	case codes.OK:
		return ClassUnknown
	case codes.Canceled:
		return ClassCanceled
	case codes.DeadlineExceeded:
		return ClassTimeout
	case codes.Unavailable:
		return ClassNetwork
	case codes.Unauthenticated, codes.PermissionDenied:
		return ClassAuth
	default:
		return ClassProtocol
	}
}

func isTLSError(err error) bool {
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.CertificateInvalidError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.SystemRootsError](err); ok {
		return true
	}
	_, ok := errors.AsType[tls.RecordHeaderError](err)
	return ok
}

func isTransportFailure(class Class) bool {
	switch class {
	case ClassNetwork, ClassTLS, ClassTimeout, ClassCanceled:
		return true
	default:
		return false
	}
}
