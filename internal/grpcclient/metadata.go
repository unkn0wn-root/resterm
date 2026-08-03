package grpcclient

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc/metadata"
)

type metaSrc string

const (
	metaSrcMeta   metaSrc = "metadata"
	metaSrcHeader metaSrc = "headers"
)

type mdPairs []string

func (p mdPairs) attach(ctx context.Context) context.Context {
	if len(p) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, p...)
}

func collectMetadata(grpcReq *restfile.GRPCRequest, req *restfile.Request) (mdPairs, error) {
	pairs, err := appendMetaPairs(nil, grpcReq.Metadata)
	if err != nil {
		return nil, err
	}
	return appendHeaderPairs(pairs, req.Headers)
}

func ValidateMetaPairs(meta []restfile.MetadataPair) error {
	_, err := appendMetaPairs(nil, meta)
	return err
}

func ValidateHeaderPairs(h http.Header) error {
	_, err := appendHeaderPairs(nil, h)
	return err
}

func appendMetaPairs(pairs mdPairs, meta []restfile.MetadataPair) (mdPairs, error) {
	for _, pair := range meta {
		key, err := normalizeMetaKey(pair.Key, metaSrcMeta)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, key, pair.Value)
	}
	return pairs, nil
}

// Sort map-backed headers so repeated runs produce metadata in the same order.
func appendHeaderPairs(pairs mdPairs, hdr http.Header) (mdPairs, error) {
	for _, key := range slices.Sorted(maps.Keys(hdr)) {
		norm, err := normalizeMetaKey(key, metaSrcHeader)
		if err != nil {
			return nil, err
		}
		for _, value := range hdr[key] {
			pairs = append(pairs, norm, value)
		}
	}
	return pairs, nil
}

func normalizeMetaKey(key string, src metaSrc) (string, error) {
	if key == "" {
		return "", metaKeyErr(src, "<empty>", "is empty")
	}
	norm := strings.ToLower(key)
	if !validMetaKey(norm) {
		return "", metaKeyErr(
			src,
			key,
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
		return diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("grpc metadata key %q from headers %s", key, msg),
			grpcComponent,
		)
	}
	return diag.New(
		diag.ClassProtocol,
		fmt.Sprintf("grpc metadata key %q %s", key, msg),
		grpcComponent,
	)
}

func validMetaKey(key string) bool {
	for _, c := range []byte(key) {
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

// reservedMetaKeys are HTTP/2 and gRPC pseudo headers that callers must not set
// as user metadata. gRPC manages them on the wire.
var reservedMetaKeys = map[string]struct{}{
	"content-type":      {},
	"user-agent":        {},
	"te":                {},
	"authority":         {},
	"host":              {},
	"connection":        {},
	"keep-alive":        {},
	"proxy-connection":  {},
	"transfer-encoding": {},
	"upgrade":           {},
}

func isReservedMetaKey(key string) bool {
	if strings.HasPrefix(key, "grpc-") || strings.HasPrefix(key, ":") {
		return true
	}
	_, reserved := reservedMetaKeys[key]
	return reserved
}
