package generator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/unkn0wn-root/resterm/internal/openapi"
	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Builder struct {
	example  *ExampleBuilder
	globals  map[string]restfile.Variable
	warnings []string
}

const (
	globalOAuthClientIDVar     = "oauth.clientId"
	globalOAuthClientSecretVar = "oauth.clientSecret"
	globalOAuthScopeVar        = "oauth.scope"
	globalOAuthUsernameVar     = "oauth.username"
	globalOAuthPasswordVar     = "oauth.password"
	globalAuthUsernameVar      = "auth.username"
	globalAuthPasswordVar      = "auth.password"
	globalAuthTokenVar         = "auth.token"
	globalAuthAPIKeyVar        = "auth.apiKey"

	placeholderClientID     = "replace-with-client-id"
	placeholderClientSecret = "replace-with-client-secret"
	placeholderPassword     = "replace-with-password"
	placeholderToken        = "replace-with-token"
	placeholderAPIKey       = "replace-with-api-key"
)

func NewBuilder() *Builder {
	return &Builder{example: NewExampleBuilder(), globals: make(map[string]restfile.Variable)}
}

func (b *Builder) Generate(
	ctx context.Context,
	spec *model.Spec,
	opts openapi.GeneratorOptions,
) (*restfile.Document, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if spec == nil {
		return nil, errors.New("openapi: spec is nil")
	}

	b.globals = make(map[string]restfile.Variable)
	b.warnings = nil

	baseVar := opts.BaseURLVariable
	if strings.TrimSpace(baseVar) == "" {
		baseVar = openapi.DefaultBaseURLVariable
	}

	baseURL := selectBaseURL(spec, opts.PreferredServerIndex)
	doc := &restfile.Document{}
	if baseURL != "" {
		doc.Variables = append(doc.Variables, restfile.Variable{
			Name:  baseVar,
			Value: baseURL,
			Scope: restfile.ScopeFile,
		})
	}

	for _, op := range spec.Operations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if op.Method == "" || op.Path == "" {
			continue
		}
		if op.Deprecated && !opts.IncludeDeprecated {
			continue
		}
		req, err := b.buildRequest(op, spec, baseVar, baseURL)
		if err != nil {
			return nil, err
		}
		doc.Requests = append(doc.Requests, req)
	}

	if len(b.globals) > 0 {
		names := make([]string, 0, len(b.globals))
		for name := range b.globals {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			doc.Globals = append(doc.Globals, b.globals[name])
		}
	}

	return doc, nil
}

func (b *Builder) Warnings() []string {
	return append([]string(nil), b.warnings...)
}

func (b *Builder) buildRequest(
	op model.Operation,
	spec *model.Spec,
	baseVarName, globalBase string,
) (*restfile.Request, error) {
	rb := requestBuilder{
		builder:      b,
		op:           op,
		baseVarName:  baseVarName,
		globalBase:   globalBase,
		headers:      make(http.Header),
		usedVarNames: map[string]int{},
		spec:         spec,
	}
	return rb.build()
}

func (b *Builder) registerGlobal(name, value string, secret bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if _, exists := b.globals[name]; exists {
		return
	}
	b.globals[name] = restfile.Variable{
		Name:   name,
		Value:  value,
		Scope:  restfile.ScopeGlobal,
		Secret: secret,
	}
}

func (b *Builder) noteWarning(message string) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return
	}
	b.warnings = append(b.warnings, trimmed)
}

type requestBuilder struct {
	builder      *Builder
	op           model.Operation
	spec         *model.Spec
	baseVarName  string
	globalBase   string
	headers      http.Header
	usedVarNames map[string]int
	variables    []restfile.Variable
	queryParams  []paramBinding
	headerParams []paramBinding
	cookieParams []paramBinding
	pathParams   []paramBinding
}

type paramBinding struct {
	Param           model.Parameter
	VarName         string
	Style           string
	Explode         bool
	Kind            schemaKind
	SerializedValue string
}

type schemaKind int

const (
	schemaPrimitive schemaKind = iota
	schemaArray
	schemaObject
)

func (rb *requestBuilder) build() (*restfile.Request, error) {
	req := &restfile.Request{
		Metadata:  rb.buildMetadata(),
		Method:    string(rb.op.Method),
		Headers:   rb.headers,
		Settings:  make(map[string]string),
		Variables: nil,
	}

	rb.processParameters()
	url := rb.composeURL()
	req.URL = url

	rb.applyHeaderParameters()
	rb.applyCookieParameters()

	rb.applyRequestBody(req)
	rb.applyAcceptHeader(req)
	rb.applySecurity(req)

	if len(rb.variables) > 0 {
		req.Variables = append(req.Variables, rb.variables...)
	}

	return req, nil
}

func (rb *requestBuilder) buildMetadata() restfile.RequestMetadata {
	meta := restfile.RequestMetadata{}
	meta.Name = deriveRequestName(rb.op)
	if desc := composeDescription(rb.op.Summary, rb.op.Description); desc != "" {
		meta.Description = desc
	}
	if len(rb.op.Tags) > 0 {
		meta.Tags = cloneStrings(rb.op.Tags)
	}
	return meta
}

func (rb *requestBuilder) processParameters() {
	for _, param := range rb.op.Parameters {
		varName := rb.uniqueVariableName(param.Location, param.Name)
		binding := rb.buildParamBinding(param, varName)
		switch param.Location {
		case model.InPath:
			rb.pathParams = append(rb.pathParams, binding)
		case model.InQuery:
			rb.queryParams = append(rb.queryParams, binding)
		case model.InHeader:
			rb.headerParams = append(rb.headerParams, binding)
		case model.InCookie:
			rb.cookieParams = append(rb.cookieParams, binding)
		}
		rb.variables = append(rb.variables, restfile.Variable{
			Name:  binding.VarName,
			Value: binding.SerializedValue,
			Scope: restfile.ScopeRequest,
		})
	}
}

func (rb *requestBuilder) buildParamBinding(param model.Parameter, varName string) paramBinding {
	style := normalizeParamStyle(param)
	explode := resolveParamExplode(param, style)
	kind := rb.inferSchemaKind(param)
	sample := rb.parameterSample(param, kind)
	serialized := rb.serializeParamValue(param, kind, style, explode, sample)
	return paramBinding{
		Param:           param,
		VarName:         varName,
		Style:           style,
		Explode:         explode,
		Kind:            kind,
		SerializedValue: serialized,
	}
}

func (rb *requestBuilder) inferSchemaKind(param model.Parameter) schemaKind {
	schema := parameterSchema(param)
	if schema == nil {
		return schemaPrimitive
	}
	types := schema.Type.Slice()
	if len(types) == 0 {
		if schema.Items != nil {
			return schemaArray
		}
		if len(schema.Properties) > 0 || schema.AdditionalProperties.Schema != nil {
			return schemaObject
		}
		return schemaPrimitive
	}
	switch strings.ToLower(types[0]) {
	case "array":
		return schemaArray
	case "object":
		return schemaObject
	default:
		return schemaPrimitive
	}
}

func (rb *requestBuilder) parameterSample(param model.Parameter, kind schemaKind) any {
	if param.Example.HasValue {
		return param.Example.Value
	}
	if param.Schema != nil {
		if value, ok := rb.builder.example.FromSchema(param.Schema); ok {
			return value
		}
	}
	switch kind {
	case schemaArray:
		return []any{"sample"}
	case schemaObject:
		return map[string]any{"example": "value"}
	default:
		return defaultParameterValue(param)
	}
}

// OpenAPI parameter serialization has tons of formats.
// Arrays can be comma, space or pipe-separated.
// Objects can use deepObject notation (name[key]=val) or key-value pairs.
// "explode" means repeat the param name for each value vs encoding all values together.
func (rb *requestBuilder) serializeParamValue(
	param model.Parameter,
	kind schemaKind,
	style string,
	explode bool,
	sample any,
) string {
	switch kind {
	case schemaArray:
		values := ensureStringSlice(sample)
		if len(values) == 0 {
			values = []string{"sample"}
		}
		switch style {
		case "spaceDelimited":
			return strings.Join(values, " ")
		case "pipeDelimited":
			return strings.Join(values, "|")
		case "form":
			if explode {
				return joinNameValuePairs(param.Name, values, "&")
			}
			return strings.Join(values, ",")
		default:
			if explode {
				return joinNameValuePairs(param.Name, values, "&")
			}
			return strings.Join(values, ",")
		}
	case schemaObject:
		fields := ensureStringMap(sample)
		if len(fields) == 0 {
			fields = map[string]string{"example": "value"}
		}
		switch style {
		case "deepobject":
			return joinDeepObject(param.Name, fields)
		case "form":
			if explode {
				return joinObjectExplode(fields, "&")
			}
			return joinObjectKeyValueList(fields, ",")
		default:
			if explode {
				return joinObjectExplode(fields, "&")
			}
			return joinObjectKeyValueList(fields, ",")
		}
	default:
		switch v := sample.(type) {
		case nil:
			return defaultParameterValue(param)
		case string:
			return v
		default:
			return stringifyExample(sample, false)
		}
	}
}

func (rb *requestBuilder) registerOAuthCommonGlobals(params map[string]string) {
	rb.builder.registerGlobal(globalOAuthClientIDVar, placeholderClientID, false)
	rb.builder.registerGlobal(globalOAuthClientSecretVar, placeholderClientSecret, true)
	if scope, ok := params[openapi.OAuthParamScope]; ok {
		if trimmed := strings.TrimSpace(scope); trimmed != "" {
			rb.builder.registerGlobal(globalOAuthScopeVar, trimmed, false)
		}
	}
}

func (rb *requestBuilder) composeURL() string {
	path := rb.op.Path
	for _, binding := range rb.pathParams {
		placeholder := fmt.Sprintf("{%s}", binding.Param.Name)
		replacement := fmt.Sprintf("{{%s}}", binding.VarName)
		path = strings.ReplaceAll(path, placeholder, replacement)
	}

	query := buildQueryString(rb.queryParams)
	if query != "" {
		path = fmt.Sprintf("%s?%s", path, query)
	}

	operationBase := rb.operationBase()
	useGlobal := rb.globalBase != ""

	switch {
	case useGlobal:
		url := joinBaseAndPath(fmt.Sprintf("{{%s}}", rb.baseVarName), path)
		if operationBase != "" && operationBase != rb.globalBase {
			rb.variables = append(rb.variables, restfile.Variable{
				Name:  rb.baseVarName,
				Value: operationBase,
				Scope: restfile.ScopeRequest,
			})
		}
		return url
	case operationBase != "":
		return joinBaseAndPath(operationBase, path)
	default:
		return path
	}
}

func (rb *requestBuilder) applyHeaderParameters() {
	for _, binding := range rb.headerParams {
		value := fmt.Sprintf("{{%s}}", binding.VarName)
		rb.headers.Add(binding.Param.Name, value)
	}
}

func (rb *requestBuilder) applyCookieParameters() {
	if len(rb.cookieParams) == 0 {
		return
	}
	parts := make([]string, 0, len(rb.cookieParams))
	for _, binding := range rb.cookieParams {
		parts = append(parts, fmt.Sprintf("%s={{%s}}", binding.Param.Name, binding.VarName))
	}
	rb.headers.Add("Cookie", strings.Join(parts, "; "))
}

func (rb *requestBuilder) applyRequestBody(req *restfile.Request) {
	body := rb.op.RequestBody
	if body == nil || len(body.MediaTypes) == 0 {
		return
	}

	media := selectRequestMedia(body.MediaTypes)
	if media == nil {
		return
	}

	content, ok := rb.resolveMediaExample(media)
	if !ok {
		return
	}

	text := stringifyExample(content, true)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	req.Body = restfile.BodySource{
		Text:     text,
		MimeType: media.ContentType,
	}

	if req.Headers.Get("Content-Type") == "" && media.ContentType != "" {
		req.Headers.Set("Content-Type", media.ContentType)
	}
}

func (rb *requestBuilder) applyAcceptHeader(req *restfile.Request) {
	if req.Headers.Get("Accept") != "" {
		return
	}
	contentType, ok := selectResponseContentType(rb.op.Responses)
	if !ok {
		return
	}
	req.Headers.Set("Accept", contentType)
}

func (rb *requestBuilder) applySecurity(req *restfile.Request) {
	if len(rb.op.Security) == 0 || rb.spec == nil {
		return
	}
	for _, requirement := range rb.op.Security {
		if spec := rb.mapSecurity(requirement); spec != nil {
			req.Metadata.Auth = spec
			return
		}
	}
}

func (rb *requestBuilder) mapSecurity(req model.SecurityRequirement) *restfile.AuthSpec {
	if rb.spec == nil || rb.spec.SecuritySchemes == nil {
		return nil
	}
	scheme, ok := rb.spec.SecuritySchemes[req.SchemeName]
	if !ok {
		rb.builder.noteWarning(
			fmt.Sprintf(
				"request %s references unknown security scheme %s",
				rb.requestName(),
				req.SchemeName,
			),
		)
		return nil
	}
	switch scheme.Type {
	case model.SecurityHTTP:
		switch scheme.Subtype {
		case "basic":
			rb.builder.registerGlobal(globalAuthUsernameVar, "user", false)
			rb.builder.registerGlobal(globalAuthPasswordVar, "pass", true)
			return &restfile.AuthSpec{Type: "basic", Params: map[string]string{
				"username": fmt.Sprintf("{{%s}}", globalAuthUsernameVar),
				"password": fmt.Sprintf("{{%s}}", globalAuthPasswordVar),
			}}
		case "bearer":
			rb.builder.registerGlobal(globalAuthTokenVar, placeholderToken, true)
			return &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": fmt.Sprintf("{{%s}}", globalAuthTokenVar)},
			}
		}
	case model.SecurityAPIKey:
		placement := strings.ToLower(string(scheme.In))
		params := map[string]string{
			"placement": placement,
			"name":      scheme.Name,
			"value":     fmt.Sprintf("{{%s}}", globalAuthAPIKeyVar),
		}
		if placement == "" {
			params["placement"] = "header"
		}
		if params["name"] == "" {
			params["name"] = "X-API-Key"
		}
		rb.builder.registerGlobal(globalAuthAPIKeyVar, placeholderAPIKey, true)
		return &restfile.AuthSpec{Type: "apikey", Params: params}
	case model.SecurityOAuth2:
		return rb.buildOAuthAuthSpec(scheme, req)
	}
	rb.builder.noteWarning(
		fmt.Sprintf(
			"request %s references unsupported security scheme type %s",
			rb.requestName(),
			scheme.Type,
		),
	)
	return nil
}

func (rb *requestBuilder) buildOAuthAuthSpec(
	scheme model.SecurityScheme,
	requirement model.SecurityRequirement,
) *restfile.AuthSpec {
	flow := selectOAuthFlow(scheme)
	if flow == nil {
		rb.builder.noteWarning(
			fmt.Sprintf("request %s uses OAuth2 scheme without supported flow", rb.requestName()),
		)
		return nil
	}

	scopes := cloneStrings(requirement.Scopes)
	if len(scopes) == 0 {
		scopes = cloneStrings(flow.Scopes)
	}

	switch flow.Type {
	case model.OAuthFlowClientCredentials:
		tokenURL := strings.TrimSpace(flow.TokenURL)
		if tokenURL == "" {
			rb.builder.noteWarning(
				fmt.Sprintf(
					"request %s uses oauth2 client credentials flow without token_url",
					rb.requestName(),
				),
			)
			return nil
		}
		params := map[string]string{
			openapi.OAuthParamTokenURL:     tokenURL,
			openapi.OAuthParamClientID:     fmt.Sprintf("{{%s}}", globalOAuthClientIDVar),
			openapi.OAuthParamClientSecret: fmt.Sprintf("{{%s}}", globalOAuthClientSecretVar),
			openapi.OAuthParamClientAuth:   "basic",
			openapi.OAuthParamGrant:        openapi.OAuthGrantClientCredentials,
		}
		if len(scopes) > 0 {
			params[openapi.OAuthParamScope] = strings.Join(scopes, " ")
		}
		rb.registerOAuthCommonGlobals(params)
		return &restfile.AuthSpec{Type: "oauth2", Params: params}
	case model.OAuthFlowPassword:
		tokenURL := strings.TrimSpace(flow.TokenURL)
		if tokenURL == "" {
			rb.builder.noteWarning(
				fmt.Sprintf(
					"request %s uses oauth2 password flow without token_url",
					rb.requestName(),
				),
			)
			return nil
		}
		params := map[string]string{
			openapi.OAuthParamTokenURL:     tokenURL,
			openapi.OAuthParamClientID:     fmt.Sprintf("{{%s}}", globalOAuthClientIDVar),
			openapi.OAuthParamClientSecret: fmt.Sprintf("{{%s}}", globalOAuthClientSecretVar),
			openapi.OAuthParamClientAuth:   "basic",
			openapi.OAuthParamGrant:        openapi.OAuthGrantPassword,
			openapi.OAuthParamUsername:     fmt.Sprintf("{{%s}}", globalOAuthUsernameVar),
			openapi.OAuthParamPassword:     fmt.Sprintf("{{%s}}", globalOAuthPasswordVar),
		}
		if len(scopes) > 0 {
			params[openapi.OAuthParamScope] = strings.Join(scopes, " ")
		}
		rb.builder.registerGlobal(globalOAuthUsernameVar, "user@example.com", false)
		rb.builder.registerGlobal(globalOAuthPasswordVar, placeholderPassword, true)
		rb.registerOAuthCommonGlobals(params)
		return &restfile.AuthSpec{Type: "oauth2", Params: params}
	case model.OAuthFlowAuthorizationCode:
		tokenURL := strings.TrimSpace(flow.TokenURL)
		authURL := strings.TrimSpace(flow.AuthorizationURL)
		if tokenURL == "" || authURL == "" {
			rb.builder.noteWarning(
				fmt.Sprintf(
					"request %s uses oauth2 authorization_code flow without auth/token urls",
					rb.requestName(),
				),
			)
			return nil
		}
		params := map[string]string{
			openapi.OAuthParamTokenURL:     tokenURL,
			openapi.OAuthParamAuthURL:      authURL,
			openapi.OAuthParamClientID:     fmt.Sprintf("{{%s}}", globalOAuthClientIDVar),
			openapi.OAuthParamClientSecret: fmt.Sprintf("{{%s}}", globalOAuthClientSecretVar),
			openapi.OAuthParamGrant:        openapi.OAuthGrantAuthorizationCode,
			openapi.OAuthParamCodeMethod:   "s256",
		}
		if len(scopes) > 0 {
			params[openapi.OAuthParamScope] = strings.Join(scopes, " ")
		}
		rb.registerOAuthCommonGlobals(params)
		return &restfile.AuthSpec{Type: "oauth2", Params: params}
	case model.OAuthFlowImplicit:
		rb.builder.registerGlobal(globalAuthTokenVar, placeholderToken, true)
		rb.builder.noteWarning(
			fmt.Sprintf(
				"request %s uses unsupported oauth flow %s; generated bearer token placeholder",
				rb.requestName(),
				flow.Type,
			),
		)
		return &restfile.AuthSpec{
			Type:   "bearer",
			Params: map[string]string{"token": fmt.Sprintf("{{%s}}", globalAuthTokenVar)},
		}
	default:
		rb.builder.noteWarning(
			fmt.Sprintf("request %s uses unsupported oauth flow %s", rb.requestName(), flow.Type),
		)
		return nil
	}
}

func (rb *requestBuilder) requestName() string {
	if rb.op.ID != "" {
		return rb.op.ID
	}
	return deriveRequestName(rb.op)
}

func (rb *requestBuilder) operationBase() string {
	if len(rb.op.Servers) > 0 {
		return rb.op.Servers[0].URL
	}
	return rb.globalBase
}

func (rb *requestBuilder) uniqueVariableName(location model.ParameterLocation, name string) string {
	base := sanitizeVariableName(location, name)
	count := rb.usedVarNames[base]
	rb.usedVarNames[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, count+1)
}

func (rb *requestBuilder) resolveMediaExample(media *model.MediaType) (any, bool) {
	if media.Example.HasValue {
		return media.Example.Value, true
	}
	if media.Schema != nil {
		return rb.builder.example.FromSchema(media.Schema)
	}
	return nil, false
}

func normalizeParamStyle(param model.Parameter) string {
	style := strings.ToLower(strings.TrimSpace(param.Style))
	if style != "" {
		return style
	}
	switch param.Location {
	case model.InQuery, model.InCookie:
		return "form"
	case model.InPath, model.InHeader:
		return "simple"
	default:
		return "form"
	}
}

func resolveParamExplode(param model.Parameter, style string) bool {
	if param.Explode != nil {
		return *param.Explode
	}
	return style == "form"
}

func parameterSchema(param model.Parameter) *openapi3.Schema {
	if param.Schema == nil {
		return nil
	}
	ref, ok := param.Schema.Payload.(*openapi3.SchemaRef)
	if !ok || ref == nil {
		return nil
	}
	if ref.Value != nil {
		return ref.Value
	}
	return nil
}

func ensureStringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case nil:
		return nil
	default:
		return []string{fmt.Sprint(v)}
	}
}

func ensureStringMap(value any) map[string]string {
	result := make(map[string]string)
	switch v := value.(type) {
	case map[string]string:
		for key, val := range v {
			result[key] = val
		}
	case map[string]any:
		for key, val := range v {
			result[key] = fmt.Sprint(val)
		}
	}
	return result
}

func joinNameValuePairs(name string, values []string, sep string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "value"
	}

	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%s=%s", name, strings.TrimSpace(value)))
	}
	return strings.Join(parts, sep)
}

func joinObjectExplode(fields map[string]string, sep string) string {
	if len(fields) == 0 {
		return ""
	}

	keys := sortedFieldKeys(fields)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, strings.TrimSpace(fields[key])))
	}
	return strings.Join(parts, sep)
}

func joinObjectKeyValueList(fields map[string]string, sep string) string {
	if len(fields) == 0 {
		return ""
	}

	keys := sortedFieldKeys(fields)
	parts := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		parts = append(parts, key, strings.TrimSpace(fields[key]))
	}
	return strings.Join(parts, sep)
}

func joinDeepObject(name string, fields map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "object"
	}

	keys := sortedFieldKeys(fields)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s[%s]=%s", name, key, strings.TrimSpace(fields[key])))
	}
	return strings.Join(parts, "&")
}

func sortedFieldKeys(fields map[string]string) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func selectBaseURL(spec *model.Spec, preferred int) string {
	if len(spec.Servers) > 0 {
		idx := preferred
		if idx < 0 || idx >= len(spec.Servers) {
			idx = 0
		}
		return spec.Servers[idx].URL
	}
	for _, op := range spec.Operations {
		if len(op.Servers) > 0 {
			return op.Servers[0].URL
		}
	}
	return ""
}

// Picks client_credentials or password flow over the browser-based ones
// since those need manual user interaction that can't be automated in a cli.
func selectOAuthFlow(scheme model.SecurityScheme) *model.OAuthFlow {
	if len(scheme.OAuthFlows) == 0 {
		return nil
	}
	order := []model.OAuthFlowType{
		model.OAuthFlowClientCredentials,
		model.OAuthFlowPassword,
		model.OAuthFlowAuthorizationCode,
		model.OAuthFlowImplicit,
	}
	for _, target := range order {
		for i := range scheme.OAuthFlows {
			flow := &scheme.OAuthFlows[i]
			if flow.Type == target {
				return flow
			}
		}
	}
	return nil
}

func selectRequestMedia(media []model.MediaType) *model.MediaType {
	if len(media) == 0 {
		return nil
	}
	for _, mt := range media {
		if strings.EqualFold(mt.ContentType, "application/json") {
			return &mt
		}
	}
	return &media[0]
}

func selectResponseContentType(responses []model.Response) (string, bool) {
	if len(responses) == 0 {
		return "", false
	}
	bestScore := -1
	selected := ""
	for _, resp := range responses {
		if len(resp.MediaTypes) == 0 {
			continue
		}
		statusScore := responseStatusScore(resp.StatusCode)
		for _, mt := range resp.MediaTypes {
			score := statusScore
			if strings.EqualFold(mt.ContentType, "application/json") {
				score += 10
			}
			if score > bestScore {
				bestScore = score
				selected = mt.ContentType
			}
		}
	}
	if selected == "" {
		return "", false
	}
	return selected, true
}

// 2xx responses score highest (200 gets 100, 299 gets 51).
// Other numeric codes get 10, "default" gets 1.
// Lower codes within 2xx range win so we prefer 200 over 201.
func responseStatusScore(code string) int {
	if code == "default" {
		return 1
	}
	if len(code) == 3 {
		if numeric, err := strconv.Atoi(code); err == nil {
			if numeric >= 200 && numeric < 300 {
				return 50 + (300 - numeric)
			}
			return 10
		}
	}
	return 0
}

func buildQueryString(params []paramBinding) string {
	if len(params) == 0 {
		return ""
	}
	sort.Slice(params, func(i, j int) bool {
		return params[i].Param.Name < params[j].Param.Name
	})
	var segments []string
	for _, binding := range params {
		segments = append(segments, serializeQueryBinding(binding)...)
	}
	return strings.Join(segments, "&")
}

// For "exploded" arrays/objects, the variable holds the pre-serialized query string
// so we emit just the variable. Otherwise wrap it in name=value format.
func serializeQueryBinding(binding paramBinding) []string {
	name := strings.TrimSpace(binding.Param.Name)
	if name == "" {
		name = "param"
	}
	style := strings.ToLower(strings.TrimSpace(binding.Style))
	switch binding.Kind {
	case schemaArray:
		if style == "form" && binding.Explode {
			return []string{fmt.Sprintf("{{%s}}", binding.VarName)}
		}
		return []string{fmt.Sprintf("%s={{%s}}", name, binding.VarName)}
	case schemaObject:
		switch style {
		case "deepobject":
			return []string{fmt.Sprintf("{{%s}}", binding.VarName)}
		case "form":
			if binding.Explode {
				return []string{fmt.Sprintf("{{%s}}", binding.VarName)}
			}
		}
		return []string{fmt.Sprintf("%s={{%s}}", name, binding.VarName)}
	default:
		return []string{fmt.Sprintf("%s={{%s}}", name, binding.VarName)}
	}
}

func joinBaseAndPath(base, path string) string {
	if base == "" {
		return path
	}
	trimmedBase := strings.TrimRight(base, "/")
	if path == "" {
		return trimmedBase
	}
	if strings.HasPrefix(path, "/") {
		return trimmedBase + path
	}
	return trimmedBase + "/" + path
}

func deriveRequestName(op model.Operation) string {
	if op.ID != "" {
		return op.ID
	}
	b := strings.Builder{}
	b.WriteString(strings.ToLower(string(op.Method)))
	segments := strings.Split(strings.Trim(op.Path, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		clean := sanitizeSegmentForName(segment)
		if clean == "" {
			continue
		}
		b.WriteString(capitalize(clean))
	}
	return b.String()
}

func sanitizeSegmentForName(segment string) string {
	segment = strings.Trim(segment, "{}")
	var builder strings.Builder
	for _, r := range segment {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

func capitalize(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func composeDescription(summary, description string) string {
	summary = strings.TrimSpace(summary)
	description = strings.TrimSpace(description)
	switch {
	case summary == "":
		return description
	case description == "":
		return summary
	default:
		return summary + "\n" + description
	}
}

func sanitizeVariableName(location model.ParameterLocation, name string) string {
	var builder strings.Builder
	prefix := strings.ToLower(string(location))
	if prefix == "" {
		prefix = "param"
	}
	builder.WriteString(prefix)
	builder.WriteRune('_')

	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(unicode.ToLower(r))
		default:
			builder.WriteRune('_')
		}
	}

	sanitized := builder.String()
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		sanitized = prefix + "_value"
	}
	if r := []rune(sanitized); len(r) > 0 && !unicode.IsLetter(r[0]) {
		sanitized = "v_" + sanitized
	}
	return sanitized
}

func stringifyExample(value any, pretty bool) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case json.Marshaler:
		data, err := v.MarshalJSON()
		if err != nil {
			break
		}
		return string(data)
	case float64, float32, int, int32, int64, uint, uint64, bool:
		return fmt.Sprint(v)
	}

	var data []byte
	var err error
	if pretty {
		data, err = json.MarshalIndent(value, "", "  ")
	} else {
		data, err = json.Marshal(value)
	}
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func defaultParameterValue(param model.Parameter) string {
	switch param.Location {
	case model.InPath:
		return "sample"
	case model.InQuery:
		return ""
	case model.InHeader:
		return ""
	case model.InCookie:
		return ""
	default:
		return ""
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
