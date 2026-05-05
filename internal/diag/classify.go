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

func classify(err error) Class {
	if err == nil {
		return ClassUnknown
	}
	if e, ok := err.(*diagnosticError); ok {
		return classFromError(e)
	}
	if rep, ok := err.(reporter); ok {
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
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ClassTimeout
	}
	if st, ok := status.FromError(err); ok {
		if class := grpcStatusClass(st.Code()); class != ClassUnknown {
			return class
		}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if class := classify(urlErr.Err); class != ClassUnknown {
			return class
		}
		return ClassNetwork
	}
	if isTLSError(err) {
		return ClassTLS
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return ClassNetwork
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return ClassNetwork
	}
	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, fs.ErrPermission),
		errors.Is(err, fs.ErrExist),
		errors.Is(err, fs.ErrClosed):
		return ClassFilesystem
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return ClassFilesystem
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		return dominantClass(multi.Unwrap())
	}
	if single, ok := err.(interface{ Unwrap() error }); ok {
		return classify(single.Unwrap())
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
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return true
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return true
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		return true
	}
	var roots x509.SystemRootsError
	if errors.As(err, &roots) {
		return true
	}
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &recordHeader) {
		return true
	}
	return false
}

func isTransportFailure(class Class) bool {
	switch class {
	case ClassNetwork, ClassTLS, ClassTimeout, ClassCanceled:
		return true
	default:
		return false
	}
}
