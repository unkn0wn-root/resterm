package httpx

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

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

type AuthPlan struct {
	Values  []AuthValue
	Targets []string
}

func (p *AuthPlan) claim(name string) {
	p.Targets = append(p.Targets, name)
}

func (p *AuthPlan) header(name, value string) {
	p.Values = append(p.Values, AuthValue{Placement: AuthInHeader, Name: name, Value: value})
}

func (p *AuthPlan) query(name, value string) {
	p.Values = append(p.Values, AuthValue{Placement: AuthInQuery, Name: name, Value: value})
}

func ResolveAuth(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
	existing http.Header,
	component diag.Component,
) (AuthPlan, error) {
	var plan AuthPlan
	if auth == nil || len(auth.Params) == 0 {
		return plan, nil
	}

	kind := auth.Kind()
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
	switch kind {
	case restfile.AuthBasic:
		plan.claim(authorizationHeader)
		if header.Present(existing, authorizationHeader) {
			return plan, nil
		}
		user, err := expand(authParamUsername)
		if err != nil {
			return AuthPlan{}, err
		}
		pass, err := expand(authParamPassword)
		if err != nil {
			return AuthPlan{}, err
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		plan.header(authorizationHeader, basicPrefix+encoded)

	case restfile.AuthBearer:
		plan.claim(authorizationHeader)
		if header.Present(existing, authorizationHeader) {
			return plan, nil
		}
		token, err := expand(authParamToken)
		if err != nil {
			return AuthPlan{}, err
		}
		plan.header(authorizationHeader, bearerTokenPrefix+token)

	case restfile.AuthAPIKey:
		placement, err := expandResult(authParamPlacement)
		if err != nil {
			return AuthPlan{}, err
		}
		name, err := expand(authParamName)
		if err != nil {
			return AuthPlan{}, err
		}
		switch util.LowerTrim(placement.Value) {
		case authPlacementQuery:
			value, err := expand(authParamValue)
			if err != nil {
				return AuthPlan{}, err
			}
			plan.query(name, value)
		case "", authPlacementHeader:
			if name == "" {
				name = defaultAPIKeyHeader
			}
			plan.claim(name)
			if header.Present(existing, name) {
				return plan, nil
			}
			value, err := expand(authParamValue)
			if err != nil {
				return AuthPlan{}, err
			}
			plan.header(name, value)
		default:
			if placement.HasUndefinedVariables {
				return AuthPlan{}, nil
			}
			msg := fmt.Sprintf(
				"invalid apikey auth placement %q, expected header or query",
				placement.Value,
			)
			if at := auth.Origin(); at != "" {
				msg += " (" + at + ")"
			}
			return AuthPlan{}, diag.New(diag.ClassAuth, msg, diag.WithComponent(component))
		}

	case restfile.AuthHeader:
		name, err := expand(authParamHeader)
		if err != nil {
			return AuthPlan{}, err
		}
		if name == "" {
			return plan, nil
		}
		plan.claim(name)
		if header.Present(existing, name) {
			return plan, nil
		}
		value, err := expand(authParamValue)
		if err != nil {
			return AuthPlan{}, err
		}
		plan.header(name, value)
	}
	return plan, nil
}
