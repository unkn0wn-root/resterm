package grpcclient

import (
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type metaSrc string

const (
	metaSrcMeta   metaSrc = "metadata"
	metaSrcHeader metaSrc = "headers"
)

func collectMetadata(grpcReq *restfile.GRPCRequest, req *restfile.Request) ([]string, error) {
	pairs := []string{}
	if grpcReq != nil && len(grpcReq.Metadata) > 0 {
		var err error
		pairs, err = appendMetaPairs(pairs, grpcReq.Metadata, metaSrcMeta)
		if err != nil {
			return nil, err
		}
	}

	if req != nil && len(req.Headers) > 0 {
		var err error
		pairs, err = appendHeaderPairs(pairs, req.Headers, metaSrcHeader)
		if err != nil {
			return nil, err
		}
	}
	return pairs, nil
}

func ValidateMetaPairs(meta []restfile.MetadataPair) error {
	_, err := appendMetaPairs(nil, meta, metaSrcMeta)
	return err
}

func ValidateHeaderPairs(h http.Header) error {
	_, err := appendHeaderPairs(nil, h, metaSrcHeader)
	return err
}

func appendMetaPairs(
	pairs []string,
	meta []restfile.MetadataPair,
	src metaSrc,
) ([]string, error) {
	for _, pair := range meta {
		key, err := normalizeMetaKey(pair.Key, src)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, key, pair.Value)
	}
	return pairs, nil
}

func appendHeaderPairs(pairs []string, hdr http.Header, src metaSrc) ([]string, error) {
	for key, values := range hdr {
		norm, err := normalizeMetaKey(key, src)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			pairs = append(pairs, norm, value)
		}
	}
	return pairs, nil
}

func normalizeMetaKey(key string, src metaSrc) (string, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", metaKeyErr(src, "<empty>", "is empty")
	}
	norm := strings.ToLower(trimmed)
	if !validMetaKey(norm) {
		return "", metaKeyErr(
			src,
			trimmed,
			"has invalid characters; allowed: a-z, 0-9, '-', '_', '.'",
		)
	}
	if isReservedMetaKey(norm) {
		if norm == "grpc-timeout" {
			return "", metaKeyErr(
				src,
				norm,
				"is reserved; use @timeout or @setting timeout",
			)
		}
		return "", metaKeyErr(src, norm, "is reserved")
	}
	return norm, nil
}

func metaKeyErr(src metaSrc, key string, msg string) error {
	if src == metaSrcHeader {
		return errdef.New(errdef.CodeHTTP, "grpc metadata key %q from headers %s", key, msg)
	}
	return errdef.New(errdef.CodeHTTP, "grpc metadata key %q %s", key, msg)
}

func validMetaKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c >= 'a' && c <= 'z' {
			continue
		}
		if c >= '0' && c <= '9' {
			continue
		}
		switch c {
		case '-', '_', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func isReservedMetaKey(key string) bool {
	if strings.HasPrefix(key, "grpc-") || strings.HasPrefix(key, ":") {
		return true
	}
	switch key {
	case "content-type",
		"user-agent",
		"te",
		"authority",
		"host",
		"connection",
		"keep-alive",
		"proxy-connection",
		"transfer-encoding",
		"upgrade":
		return true
	default:
		return false
	}
}
