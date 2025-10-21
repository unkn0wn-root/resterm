package model

type Spec struct {
	Title           string
	Version         string
	Description     string
	Servers         []Server
	Operations      []Operation
	SecuritySchemes map[string]SecurityScheme
}

type Server struct {
	URL         string
	Description string
}

type HTTPMethod string

const (
	MethodGet     HTTPMethod = "GET"
	MethodPost    HTTPMethod = "POST"
	MethodPut     HTTPMethod = "PUT"
	MethodPatch   HTTPMethod = "PATCH"
	MethodDelete  HTTPMethod = "DELETE"
	MethodHead    HTTPMethod = "HEAD"
	MethodOptions HTTPMethod = "OPTIONS"
	MethodTrace   HTTPMethod = "TRACE"
)

type Operation struct {
	ID          string
	Method      HTTPMethod
	Path        string
	Summary     string
	Description string
	Tags        []string
	Deprecated  bool
	Servers     []Server
	Parameters  []Parameter
	RequestBody *RequestBody
	Responses   []Response
	Security    []SecurityRequirement
}

type ParameterLocation string

const (
	InPath   ParameterLocation = "path"
	InQuery  ParameterLocation = "query"
	InHeader ParameterLocation = "header"
	InCookie ParameterLocation = "cookie"
)

type Parameter struct {
	Name        string
	Location    ParameterLocation
	Description string
	Required    bool
	Style       string
	Explode     *bool
	Example     Example
	Schema      *SchemaRef
}

type RequestBody struct {
	Description string
	Required    bool
	MediaTypes  []MediaType
}

type Response struct {
	StatusCode  string
	Description string
	MediaTypes  []MediaType
}

type MediaType struct {
	ContentType string
	Example     Example
	Schema      *SchemaRef
}

type ExampleSource string

const (
	ExampleFromExplicit ExampleSource = "explicit"
	ExampleFromDefault  ExampleSource = "default"
	ExampleFromEnum     ExampleSource = "enum"
	ExampleFromSchema   ExampleSource = "schema"
)

type Example struct {
	Summary  string
	Value    any
	Source   ExampleSource
	HasValue bool
}

type SchemaRef struct {
	Identifier string
	Payload    any
}

type SecuritySchemeType string

const (
	SecurityHTTP   SecuritySchemeType = "http"
	SecurityAPIKey SecuritySchemeType = "apiKey"
	SecurityOAuth2 SecuritySchemeType = "oauth2"
	SecurityOpenID SecuritySchemeType = "openIdConnect"
)

type SecurityScheme struct {
	Type         SecuritySchemeType
	Subtype      string
	Name         string
	In           ParameterLocation
	Description  string
	BearerFormat string
	OAuthFlows   []OAuthFlow
}

type SecurityRequirement struct {
	SchemeName string
	Scopes     []string
}

type OAuthFlowType string

const (
	OAuthFlowAuthorizationCode OAuthFlowType = "authorizationCode"
	OAuthFlowImplicit          OAuthFlowType = "implicit"
	OAuthFlowPassword          OAuthFlowType = "password"
	OAuthFlowClientCredentials OAuthFlowType = "clientCredentials"
)

type OAuthFlow struct {
	Type             OAuthFlowType
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           []string
}
