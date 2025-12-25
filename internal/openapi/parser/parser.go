package parser

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

type Loader struct{}

func NewLoader() *Loader {
	return &Loader{}
}

func (l *Loader) Parse(
	ctx context.Context,
	path string,
	opts openapi.ParseOptions,
) (*model.Spec, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = opts.ResolveExternalRefs

	document, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}

	if err := document.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate OpenAPI spec: %w", err)
	}

	spec := &model.Spec{
		Title:           document.Info.Title,
		Version:         document.Info.Version,
		Description:     document.Info.Description,
		Servers:         convertServers(document.Servers),
		SecuritySchemes: convertSecuritySchemes(document.Components.SecuritySchemes),
	}

	operations := collectOperations(document)
	spec.Operations = operations

	return spec, nil
}

func collectOperations(doc *openapi3.T) []model.Operation {
	if doc.Paths == nil {
		return nil
	}

	pathMap := doc.Paths.Map()
	paths := make([]string, 0, len(pathMap))
	for path := range pathMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var ops []model.Operation
	for _, path := range paths {
		item := doc.Paths.Value(path)
		if item == nil {
			continue
		}

		baseParameters := item.Parameters
		pathServers := item.Servers
		methodOrder := []struct {
			method model.HTTPMethod
			op     *openapi3.Operation
		}{
			{model.MethodGet, item.Get},
			{model.MethodPut, item.Put},
			{model.MethodPost, item.Post},
			{model.MethodDelete, item.Delete},
			{model.MethodOptions, item.Options},
			{model.MethodHead, item.Head},
			{model.MethodPatch, item.Patch},
			{model.MethodTrace, item.Trace},
		}

		for _, entry := range methodOrder {
			if entry.op == nil {
				continue
			}
			op := normalizeOperation(doc, path, entry.method, entry.op, baseParameters, pathServers)
			ops = append(ops, op)
		}
	}
	return ops
}

func normalizeOperation(
	doc *openapi3.T,
	path string,
	method model.HTTPMethod,
	raw *openapi3.Operation,
	baseParams openapi3.Parameters,
	pathServers openapi3.Servers,
) model.Operation {
	op := model.Operation{
		ID:          raw.OperationID,
		Method:      method,
		Path:        path,
		Summary:     raw.Summary,
		Description: raw.Description,
		Tags:        cloneStringSlice(raw.Tags),
		Deprecated:  raw.Deprecated,
		Servers:     convertServers(selectServers(doc.Servers, pathServers, raw.Servers)),
	}

	op.Parameters = mergeParameters(baseParams, raw.Parameters)
	op.RequestBody = convertRequestBody(raw.RequestBody)
	op.Responses = convertResponses(raw.Responses)
	op.Security = resolveSecurityRequirements(doc, raw)

	return op
}

func convertServers(servers openapi3.Servers) []model.Server {
	if len(servers) == 0 {
		return nil
	}

	result := make([]model.Server, 0, len(servers))
	for _, srv := range servers {
		if srv == nil {
			continue
		}
		resolvedURL := resolveServerURL(srv)
		result = append(result, model.Server{
			URL:         resolvedURL,
			Description: srv.Description,
		})
	}
	return result
}

func selectServers(
	docServers openapi3.Servers,
	pathServers openapi3.Servers,
	opServers *openapi3.Servers,
) openapi3.Servers {
	if opServers != nil && len(*opServers) > 0 {
		return *opServers
	}
	if len(pathServers) > 0 {
		return pathServers
	}
	if len(docServers) > 0 {
		return docServers
	}
	return nil
}

func resolveServerURL(server *openapi3.Server) string {
	if server == nil {
		return ""
	}
	if len(server.Variables) == 0 {
		return server.URL
	}
	resolved := server.URL
	for name, variable := range server.Variables {
		replacement := variable.Default
		if replacement == "" && len(variable.Enum) > 0 {
			replacement = variable.Enum[0]
		}
		placeholder := fmt.Sprintf("{%s}", name)
		resolved = strings.ReplaceAll(resolved, placeholder, replacement)
	}
	return resolved
}

func mergeParameters(baseParams, opParams openapi3.Parameters) []model.Parameter {
	combined := make(map[string]model.Parameter)

	addParam := func(ref *openapi3.ParameterRef) {
		if ref == nil {
			return
		}
		if ref.Value == nil {
			return
		}
		key := ref.Value.In + ":" + ref.Value.Name
		combined[key] = convertParameter(ref.Value)
	}

	for _, ref := range baseParams {
		addParam(ref)
	}

	for _, ref := range opParams {
		addParam(ref)
	}

	if len(combined) == 0 {
		return nil
	}

	keys := make([]string, 0, len(combined))
	for key := range combined {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	params := make([]model.Parameter, 0, len(keys))
	for _, key := range keys {
		params = append(params, combined[key])
	}
	return params
}

func convertParameter(p *openapi3.Parameter) model.Parameter {
	param := model.Parameter{
		Name:        p.Name,
		Location:    model.ParameterLocation(p.In),
		Description: p.Description,
		Required:    p.Required,
		Style:       p.Style,
		Example:     extractParameterExample(p),
	}

	if p.Explode != nil {
		value := *p.Explode
		param.Explode = &value
	}

	if p.Schema != nil {
		param.Schema = &model.SchemaRef{Identifier: p.Schema.Ref, Payload: p.Schema}
		if !param.Example.HasValue {
			if ex, ok := extractExampleFromSchema(p.Schema); ok {
				param.Example = ex
			}
		}
	}

	return param
}

func extractParameterExample(p *openapi3.Parameter) model.Example {
	if p.Example != nil {
		return model.Example{Value: p.Example, Source: model.ExampleFromExplicit, HasValue: true}
	}

	if len(p.Examples) > 0 {
		for _, exRef := range p.Examples {
			if exRef != nil && exRef.Value != nil {
				return model.Example{
					Summary:  exRef.Value.Summary,
					Value:    exRef.Value.Value,
					Source:   model.ExampleFromExplicit,
					HasValue: true,
				}
			}
		}
	}

	return model.Example{}
}

func extractExampleFromSchema(ref *openapi3.SchemaRef) (model.Example, bool) {
	if ref == nil || ref.Value == nil {
		return model.Example{}, false
	}

	schema := ref.Value
	if schema.Example != nil {
		return model.Example{
			Value:    schema.Example,
			Source:   model.ExampleFromExplicit,
			HasValue: true,
		}, true
	}

	if schema.Default != nil {
		return model.Example{
			Value:    schema.Default,
			Source:   model.ExampleFromDefault,
			HasValue: true,
		}, true
	}

	if len(schema.Enum) > 0 {
		return model.Example{
			Value:    schema.Enum[0],
			Source:   model.ExampleFromEnum,
			HasValue: true,
		}, true
	}

	return model.Example{}, false
}

func convertRequestBody(ref *openapi3.RequestBodyRef) *model.RequestBody {
	if ref == nil || ref.Value == nil {
		return nil
	}

	rb := ref.Value
	result := &model.RequestBody{
		Description: rb.Description,
		Required:    rb.Required,
	}

	if len(rb.Content) == 0 {
		return result
	}

	mediaTypes := make([]string, 0, len(rb.Content))
	for mediaType := range rb.Content {
		mediaTypes = append(mediaTypes, mediaType)
	}
	sort.Strings(mediaTypes)

	for _, mediaType := range mediaTypes {
		mt := rb.Content[mediaType]
		if mt == nil {
			continue
		}
		example := extractMediaTypeExample(mt)
		media := model.MediaType{
			ContentType: mediaType,
			Example:     example,
		}
		if mt.Schema != nil {
			media.Schema = &model.SchemaRef{Identifier: mt.Schema.Ref, Payload: mt.Schema}
			if !media.Example.HasValue {
				if ex, ok := extractExampleFromSchema(mt.Schema); ok {
					media.Example = ex
				}
			}
		}
		result.MediaTypes = append(result.MediaTypes, media)
	}

	return result
}

func extractMediaTypeExample(mt *openapi3.MediaType) model.Example {
	if mt == nil {
		return model.Example{}
	}

	if mt.Example != nil {
		return model.Example{Value: mt.Example, Source: model.ExampleFromExplicit, HasValue: true}
	}

	if len(mt.Examples) > 0 {
		for _, ref := range mt.Examples {
			if ref != nil && ref.Value != nil {
				return model.Example{
					Summary:  ref.Value.Summary,
					Value:    ref.Value.Value,
					Source:   model.ExampleFromExplicit,
					HasValue: true,
				}
			}
		}
	}

	return model.Example{}
}

func convertResponses(responses *openapi3.Responses) []model.Response {
	if responses == nil || responses.Len() == 0 {
		return nil
	}

	statusCodes := make([]string, 0, responses.Len())
	for code := range responses.Map() {
		statusCodes = append(statusCodes, code)
	}
	sort.Strings(statusCodes)

	result := make([]model.Response, 0, len(statusCodes))
	for _, code := range statusCodes {
		ref := responses.Value(code)
		if ref == nil || ref.Value == nil {
			continue
		}
		resp := model.Response{
			StatusCode:  code,
			Description: valueOrZero(ref.Value.Description),
		}

		if len(ref.Value.Content) > 0 {
			mediaTypes := make([]string, 0, len(ref.Value.Content))
			for mediaType := range ref.Value.Content {
				mediaTypes = append(mediaTypes, mediaType)
			}
			sort.Strings(mediaTypes)
			for _, mediaType := range mediaTypes {
				mt := ref.Value.Content[mediaType]
				if mt == nil {
					continue
				}
				example := extractMediaTypeExample(mt)
				media := model.MediaType{
					ContentType: mediaType,
					Example:     example,
				}
				if mt.Schema != nil {
					media.Schema = &model.SchemaRef{Identifier: mt.Schema.Ref, Payload: mt.Schema}
					if !media.Example.HasValue {
						if ex, ok := extractExampleFromSchema(mt.Schema); ok {
							media.Example = ex
						}
					}
				}
				resp.MediaTypes = append(resp.MediaTypes, media)
			}
		}
		result = append(result, resp)
	}

	return result
}

func resolveSecurityRequirements(
	doc *openapi3.T,
	op *openapi3.Operation,
) []model.SecurityRequirement {
	var source openapi3.SecurityRequirements
	if op.Security != nil {
		source = *op.Security
	} else if len(doc.Security) > 0 {
		source = doc.Security
	}
	if len(source) == 0 {
		return nil
	}

	var requirements []model.SecurityRequirement
	for _, requirement := range source {
		if len(requirement) == 0 {
			continue
		}
		keys := make([]string, 0, len(requirement))
		for scheme := range requirement {
			keys = append(keys, scheme)
		}
		sort.Strings(keys)
		for _, scheme := range keys {
			scopes := requirement[scheme]
			requirements = append(requirements, model.SecurityRequirement{
				SchemeName: scheme,
				Scopes:     cloneStringSlice(scopes),
			})
		}
	}

	return requirements
}

func convertSecuritySchemes(
	raw map[string]*openapi3.SecuritySchemeRef,
) map[string]model.SecurityScheme {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]model.SecurityScheme, len(raw))
	for name, ref := range raw {
		if ref == nil || ref.Value == nil {
			continue
		}
		scheme := ref.Value
		entry := model.SecurityScheme{
			Type:         model.SecuritySchemeType(scheme.Type),
			Subtype:      strings.ToLower(scheme.Scheme),
			Name:         scheme.Name,
			In:           model.ParameterLocation(scheme.In),
			Description:  scheme.Description,
			BearerFormat: scheme.BearerFormat,
		}
		if strings.EqualFold(scheme.Type, string(model.SecurityOAuth2)) {
			entry.OAuthFlows = convertOAuthFlows(scheme.Flows)
		}
		result[name] = entry
	}
	return result
}

func convertOAuthFlows(flows *openapi3.OAuthFlows) []model.OAuthFlow {
	if flows == nil {
		return nil
	}
	var result []model.OAuthFlow
	appendFlow := func(flow *openapi3.OAuthFlow, flowType model.OAuthFlowType) {
		if flow == nil {
			return
		}
		scopes := make([]string, 0, len(flow.Scopes))
		for scope := range flow.Scopes {
			scopes = append(scopes, scope)
		}
		sort.Strings(scopes)
		result = append(result, model.OAuthFlow{
			Type:             flowType,
			AuthorizationURL: flow.AuthorizationURL,
			TokenURL:         flow.TokenURL,
			RefreshURL:       flow.RefreshURL,
			Scopes:           scopes,
		})
	}
	appendFlow(flows.ClientCredentials, model.OAuthFlowClientCredentials)
	appendFlow(flows.Password, model.OAuthFlowPassword)
	appendFlow(flows.AuthorizationCode, model.OAuthFlowAuthorizationCode)
	appendFlow(flows.Implicit, model.OAuthFlowImplicit)
	return result
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func valueOrZero[T any](ptr *T) T {
	var zero T
	if ptr == nil {
		return zero
	}
	return *ptr
}
