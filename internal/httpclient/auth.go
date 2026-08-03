package httpclient

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// AuthPlacement identifies where a resolved auth value belongs on the wire.
type AuthPlacement int

const (
	AuthInHeader AuthPlacement = iota
	AuthInQuery
)

type AuthValue struct {
	Placement AuthPlacement
	Name      string
	Value     string
}

// AuthValues resolves auth without mutating a request. This lets HTTP and gRPC
// share expansion while the engine still registers the values for redaction.
// Existing headers are never overwritten.
func AuthValues(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	existing http.Header,
	component diag.Component,
) ([]AuthValue, error) {
	if auth == nil || len(auth.Params) == 0 {
		return nil, nil
	}

	kind := strings.ToLower(auth.Type)
	expandResult := func(param string) (vars.Expansion, error) {
		value := auth.Params[param]
		if value == "" || resolver == nil {
			return vars.Expansion{Value: value}, nil
		}
		out, err := resolver.ExpandTemplatesResult(value)
		if err != nil {
			op := fmt.Sprintf("expand %s auth %s", kind, param)
			if at := auth.Origin(); at != "" {
				op += " (" + at + ")"
			}
			return vars.Expansion{}, diag.WrapAs(
				diag.ClassAuth,
				err,
				op,
				diag.WithComponent(component),
			)
		}
		return out, nil
	}
	expand := func(param string) (string, error) {
		out, err := expandResult(param)
		return out.Value, err
	}
	header := func(name, value string) []AuthValue {
		return []AuthValue{{Placement: AuthInHeader, Name: name, Value: value}}
	}

	switch kind {
	case string(authTypeBasic):
		if headerSet(existing, authorizationHeader) {
			return nil, nil
		}
		user, err := expand(authParamUsername)
		if err != nil {
			return nil, err
		}
		pass, err := expand(authParamPassword)
		if err != nil {
			return nil, err
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		return header(authorizationHeader, basicPrefix+encoded), nil

	case string(authTypeBearer):
		if headerSet(existing, authorizationHeader) {
			return nil, nil
		}
		token, err := expand(authParamToken)
		if err != nil {
			return nil, err
		}
		return header(authorizationHeader, bearerTokenPrefix+token), nil

	case string(authTypeAPIKey), legacyAPIKeyAuthType:
		placement, err := expandResult(authParamPlacement)
		if err != nil {
			return nil, err
		}
		name, err := expand(authParamName)
		if err != nil {
			return nil, err
		}
		switch util.LowerTrim(placement.Value) {
		case authPlacementQuery:
			value, err := expand(authParamValue)
			if err != nil {
				return nil, err
			}
			return []AuthValue{{Placement: AuthInQuery, Name: name, Value: value}}, nil
		case "", authPlacementHeader:
			if name == "" {
				name = defaultAPIKeyHeader
			}
			if headerSet(existing, name) {
				return nil, nil
			}
			value, err := expand(authParamValue)
			if err != nil {
				return nil, err
			}
			return header(name, value), nil
		default:
			if placement.HasUndefinedVariables {
				return nil, nil
			}
			msg := fmt.Sprintf(
				"invalid apikey auth placement %q, expected header or query",
				placement.Value,
			)
			if at := auth.Origin(); at != "" {
				msg += " (" + at + ")"
			}
			return nil, diag.New(diag.ClassAuth, msg, diag.WithComponent(component))
		}

	case string(authTypeHeader):
		name, err := expand(authParamHeader)
		if err != nil {
			return nil, err
		}
		if name == "" || headerSet(existing, name) {
			return nil, nil
		}
		value, err := expand(authParamValue)
		if err != nil {
			return nil, err
		}
		return header(name, value), nil
	}
	return nil, nil
}

func headerSet(h http.Header, name string) bool {
	return h != nil && h.Get(name) != ""
}
