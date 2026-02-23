package parser

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	hbase "github.com/pb33f/libopenapi/datamodel/high/base"
	h3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

type Loader struct {
	ws []string
}

func NewLoader() *Loader {
	return &Loader{}
}

func (l *Loader) Warnings() []string {
	return append([]string(nil), l.ws...)
}

func (l *Loader) noteWarn(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	l.ws = append(l.ws, msg)
}

func (l *Loader) Parse(
	ctx context.Context,
	path string,
	opts openapi.ParseOptions,
) (*model.Spec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	l.ws = nil
	sm := newSchMap()
	sm.setWarn(l.noteWarn)

	doc, ws, err := loadDoc(path, opts)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.ws = append(l.ws, ws...)

	spec := &model.Spec{
		Title:       valOr(doc.Info, func(info *hbase.Info) string { return info.Title }),
		Version:     valOr(doc.Info, func(info *hbase.Info) string { return info.Version }),
		Description: valOr(doc.Info, func(info *hbase.Info) string { return info.Description }),
		Servers:     convertServers(doc.Servers),
	}

	if doc.Components != nil {
		spec.SecuritySchemes = convertSecuritySchemes(doc.Components.SecuritySchemes, l.noteWarn)
	}

	ops, err := collectOperations(ctx, doc, sm)
	if err != nil {
		return nil, err
	}
	if err := validateOps(ops); err != nil {
		return nil, fmt.Errorf("validate OpenAPI spec: %w", err)
	}

	spec.Operations = ops
	return spec, nil
}

func validateOps(ops []model.Operation) error {
	var errs []error
	for _, op := range ops {
		if len(op.Responses) > 0 {
			continue
		}
		errs = append(
			errs,
			fmt.Errorf("%s %s: operation must define at least one response", op.Method, op.Path),
		)
	}
	return errors.Join(errs...)
}

func collectOperations(ctx context.Context, doc *h3.Document, sm *schMap) ([]model.Operation, error) {
	if doc == nil || doc.Paths == nil || orderedmap.Len(doc.Paths.PathItems) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, orderedmap.Len(doc.Paths.PathItems))
	for path := range doc.Paths.PathItems.KeysFromOldest() {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var ops []model.Operation
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item := doc.Paths.PathItems.GetOrZero(path)
		if item == nil {
			continue
		}

		baseParams := item.Parameters
		pathServers := item.Servers
		entries := collectPathOps(item)
		for _, ent := range entries {
			if ent.op == nil || ent.method == "" {
				continue
			}
			op := normalizeOperation(doc, path, ent.method, ent.op, baseParams, pathServers, sm)
			ops = append(ops, op)
		}
	}
	return ops, nil
}

type opEnt struct {
	method model.HTTPMethod
	op     *h3.Operation
}

func collectPathOps(item *h3.PathItem) []opEnt {
	out := []opEnt{
		{method: model.MethodGet, op: item.Get},
		{method: model.MethodPut, op: item.Put},
		{method: model.MethodPost, op: item.Post},
		{method: model.MethodDelete, op: item.Delete},
		{method: model.MethodOptions, op: item.Options},
		{method: model.MethodHead, op: item.Head},
		{method: model.MethodPatch, op: item.Patch},
		{method: model.MethodTrace, op: item.Trace},
		{method: model.MethodQuery, op: item.Query},
	}
	out = append(out, collectExtraPathOps(item)...)
	return out
}

func collectExtraPathOps(item *h3.PathItem) []opEnt {
	names := mapKeys(item.AdditionalOperations)
	if len(names) > 0 {
		out := make([]opEnt, 0, len(names))
		for _, name := range names {
			op := item.AdditionalOperations.GetOrZero(name)
			if op == nil {
				continue
			}
			out = append(out, opEnt{method: model.HTTPMethod(name), op: op})
		}
		return out
	}

	low := item.GoLow()
	if low == nil || low.AdditionalOperations.Value == nil || low.AdditionalOperations.Value.Len() == 0 {
		return nil
	}

	names = make([]string, 0, low.AdditionalOperations.Value.Len())
	extra := make(map[string]*h3.Operation, low.AdditionalOperations.Value.Len())
	for key, ref := range low.AdditionalOperations.Value.FromOldest() {
		name := strings.ToUpper(strings.TrimSpace(key.Value))
		if name == "" || ref.Value == nil {
			continue
		}
		if _, ok := extra[name]; ok {
			continue
		}
		names = append(names, name)
		extra[name] = h3.NewOperation(ref.Value)
	}
	sort.Strings(names)

	out := make([]opEnt, 0, len(names))
	for _, name := range names {
		out = append(out, opEnt{method: model.HTTPMethod(name), op: extra[name]})
	}

	return out
}

func normalizeOperation(
	doc *h3.Document,
	path string,
	method model.HTTPMethod,
	raw *h3.Operation,
	baseParams []*h3.Parameter,
	pathServers []*h3.Server,
	sm *schMap,
) model.Operation {
	op := model.Operation{
		ID:          raw.OperationId,
		Method:      method,
		Path:        path,
		Summary:     raw.Summary,
		Description: raw.Description,
		Tags:        model.CloneStrs(raw.Tags),
		Deprecated:  boolVal(raw.Deprecated),
		Servers:     convertServers(selectServers(doc.Servers, pathServers, raw.Servers)),
	}

	op.Parameters = mergeParameters(baseParams, raw.Parameters, sm)
	op.RequestBody = convertRequestBody(raw.RequestBody, sm)
	op.Responses = convertResponses(raw.Responses, sm)
	op.Security = resolveSecurityRequirements(doc, raw)

	return op
}

func convertServers(servers []*h3.Server) []model.Server {
	if len(servers) == 0 {
		return nil
	}

	out := make([]model.Server, 0, len(servers))
	for _, srv := range servers {
		if srv == nil {
			continue
		}
		out = append(out, model.Server{
			URL:         resolveServerURL(srv),
			Description: srv.Description,
		})
	}
	return out
}

func selectServers(docServers, pathServers, opServers []*h3.Server) []*h3.Server {
	if len(opServers) > 0 {
		return opServers
	}
	if len(pathServers) > 0 {
		return pathServers
	}
	if len(docServers) > 0 {
		return docServers
	}
	return nil
}

func resolveServerURL(server *h3.Server) string {
	if server == nil {
		return ""
	}
	if orderedmap.Len(server.Variables) == 0 {
		return server.URL
	}

	out := server.URL
	keys := make([]string, 0, server.Variables.Len())
	for key := range server.Variables.KeysFromOldest() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v := server.Variables.GetOrZero(key)
		if v == nil {
			continue
		}
		rep := strings.TrimSpace(v.Default)
		if rep == "" && len(v.Enum) > 0 {
			rep = v.Enum[0]
		}
		out = strings.ReplaceAll(out, "{"+key+"}", rep)
	}
	return out
}

func mergeParameters(baseParams, opParams []*h3.Parameter, sm *schMap) []model.Parameter {
	merged := make(map[string]model.Parameter)

	add := func(p *h3.Parameter) {
		if p == nil {
			return
		}
		key := strings.ToLower(p.In) + ":" + p.Name
		merged[key] = convertParameter(p, sm)
	}

	for _, p := range baseParams {
		add(p)
	}
	for _, p := range opParams {
		add(p)
	}

	if len(merged) == 0 {
		return nil
	}

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]model.Parameter, 0, len(keys))
	for _, key := range keys {
		out = append(out, merged[key])
	}
	return out
}

func convertParameter(p *h3.Parameter, sm *schMap) model.Parameter {
	param := model.Parameter{
		Name:        p.Name,
		Location:    model.ParameterLocation(p.In),
		Description: p.Description,
		Required:    boolVal(p.Required),
		Style:       p.Style,
		Example:     extractParameterExample(p, sm),
	}

	if p.Explode != nil {
		v := *p.Explode
		param.Explode = &v
	}

	if p.Schema != nil {
		param.Schema = sm.toRef(p.Schema)
		if !param.Example.HasValue {
			if ex, ok := extractExampleFromSchema(param.Schema); ok {
				param.Example = ex
			}
		}
	}

	return param
}

func extractParameterExample(p *h3.Parameter, sm *schMap) model.Example {
	if p == nil {
		return model.Example{}
	}
	return extractExample(p.Example, p.Examples, sm, "parameter")
}

func extractExample(
	exp *yaml.Node,
	exs *orderedmap.Map[string, *hbase.Example],
	sm *schMap,
	ctx string,
) model.Example {
	if exp != nil {
		val := nodeAny(exp, sm.warn, ctx+" explicit example")
		if val != nil {
			return model.Example{
				Value:    val,
				Source:   model.ExampleFromExplicit,
				HasValue: true,
			}
		}
	}

	if orderedmap.Len(exs) == 0 {
		return model.Example{}
	}

	for _, ex := range exs.FromOldest() {
		if ex == nil {
			continue
		}
		val, ok := exampleValue(ex, sm, ctx)
		if !ok {
			continue
		}
		return model.Example{
			Summary:  ex.Summary,
			Value:    val,
			Source:   model.ExampleFromExplicit,
			HasValue: true,
		}
	}

	return model.Example{}
}

func exampleValue(ex *hbase.Example, sm *schMap, ctx string) (any, bool) {
	if ex == nil {
		return nil, false
	}

	val := func(node *yaml.Node, loc string) (any, bool) {
		if node == nil {
			return nil, false
		}
		v := nodeAny(node, sm.warn, loc)
		if v == nil {
			return nil, false
		}
		return v, true
	}

	switch {
	case ex.Value != nil:
		return val(ex.Value, ctx+" example value")
	case ex.DataValue != nil:
		return val(ex.DataValue, ctx+" data value")
	case ex.SerializedValue != "":
		return ex.SerializedValue, true
	default:
		return nil, false
	}
}

func extractExampleFromSchema(ref *model.SchemaRef) (model.Example, bool) {
	if ref == nil || ref.Node == nil {
		return model.Example{}, false
	}

	sch := ref.Node
	if sch.Example != nil {
		return model.Example{
			Value:    sch.Example,
			Source:   model.ExampleFromExplicit,
			HasValue: true,
		}, true
	}
	if sch.Default != nil {
		return model.Example{
			Value:    sch.Default,
			Source:   model.ExampleFromDefault,
			HasValue: true,
		}, true
	}
	if len(sch.Enum) > 0 {
		return model.Example{
			Value:    sch.Enum[0],
			Source:   model.ExampleFromEnum,
			HasValue: true,
		}, true
	}
	return model.Example{}, false
}

func convertRequestBody(rb *h3.RequestBody, sm *schMap) *model.RequestBody {
	if rb == nil {
		return nil
	}

	out := &model.RequestBody{
		Description: rb.Description,
		Required:    boolVal(rb.Required),
	}
	if orderedmap.Len(rb.Content) == 0 {
		return out
	}

	types := mapKeys(rb.Content)
	for _, contentType := range types {
		mt := rb.Content.GetOrZero(contentType)
		media, ok := convertMediaType(contentType, mt, sm)
		if !ok {
			continue
		}
		out.MediaTypes = append(out.MediaTypes, media)
	}
	return out
}

func extractMediaTypeExample(mt *h3.MediaType, sm *schMap) model.Example {
	if mt == nil {
		return model.Example{}
	}
	return extractExample(mt.Example, mt.Examples, sm, "media type")
}

func convertMediaType(contentType string, mt *h3.MediaType, sm *schMap) (model.MediaType, bool) {
	if mt == nil {
		return model.MediaType{}, false
	}

	media := model.MediaType{
		ContentType: contentType,
		Example:     extractMediaTypeExample(mt, sm),
	}

	ref := pickMediaSchema(mt)
	if ref != nil {
		media.Schema = sm.toRef(ref)
		if !media.Example.HasValue {
			if ex, ok := extractExampleFromSchema(media.Schema); ok {
				media.Example = ex
			}
		}
	}

	return media, true
}

func pickMediaSchema(mt *h3.MediaType) *hbase.SchemaProxy {
	if mt == nil {
		return nil
	}
	if mt.Schema != nil {
		return mt.Schema
	}
	if mt.ItemSchema != nil {
		return mt.ItemSchema
	}
	return nil
}

func convertResponses(responses *h3.Responses, sm *schMap) []model.Response {
	if responses == nil {
		return nil
	}

	var codes []string
	if orderedmap.Len(responses.Codes) > 0 {
		codes = append(codes, mapKeys(responses.Codes)...)
	}
	if responses.Default != nil {
		codes = append(codes, "default")
	}
	if len(codes) == 0 {
		return nil
	}
	sort.Strings(codes)

	out := make([]model.Response, 0, len(codes))
	for _, code := range codes {
		var raw *h3.Response
		if code == "default" {
			raw = responses.Default
		} else if responses.Codes != nil {
			raw = responses.Codes.GetOrZero(code)
		}
		if raw == nil {
			continue
		}

		resp := model.Response{
			StatusCode:  code,
			Description: raw.Description,
		}
		if orderedmap.Len(raw.Content) > 0 {
			types := mapKeys(raw.Content)
			for _, contentType := range types {
				mt := raw.Content.GetOrZero(contentType)
				media, ok := convertMediaType(contentType, mt, sm)
				if !ok {
					continue
				}
				resp.MediaTypes = append(resp.MediaTypes, media)
			}
		}
		out = append(out, resp)
	}
	return out
}

func resolveSecurityRequirements(doc *h3.Document, op *h3.Operation) []model.SecurityRequirement {
	var src []*hbase.SecurityRequirement
	if op != nil && op.Security != nil {
		src = op.Security
	} else if doc != nil && len(doc.Security) > 0 {
		src = doc.Security
	}
	if len(src) == 0 {
		return nil
	}

	var out []model.SecurityRequirement
	for _, req := range src {
		if req == nil || orderedmap.Len(req.Requirements) == 0 {
			continue
		}

		keys := mapKeys(req.Requirements)
		for _, name := range keys {
			out = append(out, model.SecurityRequirement{
				SchemeName: name,
				Scopes:     model.CloneStrs(req.Requirements.GetOrZero(name)),
			})
		}
	}
	return out
}

func convertSecuritySchemes(
	raw *orderedmap.Map[string, *h3.SecurityScheme],
	warn func(string),
) map[string]model.SecurityScheme {
	if orderedmap.Len(raw) == 0 {
		return nil
	}

	out := make(map[string]model.SecurityScheme, orderedmap.Len(raw))
	for name, sec := range raw.FromOldest() {
		if sec == nil {
			continue
		}
		entry := model.SecurityScheme{
			Type:         model.SecuritySchemeType(sec.Type),
			Subtype:      strings.ToLower(sec.Scheme),
			Name:         sec.Name,
			In:           model.ParameterLocation(sec.In),
			Description:  sec.Description,
			BearerFormat: sec.BearerFormat,
		}
		if strings.EqualFold(sec.Type, string(model.SecurityOAuth2)) {
			entry.OAuthFlows = convertOAuthFlows(sec.Flows)
			if sec.Flows != nil && sec.Flows.Device != nil {
				noteWarn(
					warn,
					fmt.Sprintf(
						"OpenAPI compatibility: oauth2 device flow in security scheme %q is not supported; flow ignored.",
						name,
					),
				)
			}
		}
		out[name] = entry
	}
	return out
}

func convertOAuthFlows(flows *h3.OAuthFlows) []model.OAuthFlow {
	if flows == nil {
		return nil
	}

	var out []model.OAuthFlow
	add := func(flow *h3.OAuthFlow, typ model.OAuthFlowType) {
		if flow == nil {
			return
		}
		scopes := mapKeys(flow.Scopes)
		out = append(out, model.OAuthFlow{
			Type:             typ,
			AuthorizationURL: flow.AuthorizationUrl,
			TokenURL:         flow.TokenUrl,
			RefreshURL:       flow.RefreshUrl,
			Scopes:           scopes,
		})
	}

	add(flows.ClientCredentials, model.OAuthFlowClientCredentials)
	add(flows.Password, model.OAuthFlowPassword)
	add(flows.AuthorizationCode, model.OAuthFlowAuthorizationCode)
	add(flows.Implicit, model.OAuthFlowImplicit)
	return out
}

func mapKeys[V any](m *orderedmap.Map[string, V]) []string {
	if orderedmap.Len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, orderedmap.Len(m))
	for key := range m.KeysFromOldest() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func boolVal(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

func valOr[T any, R any](in *T, fn func(*T) R) R {
	var zero R
	if in == nil {
		return zero
	}
	return fn(in)
}
