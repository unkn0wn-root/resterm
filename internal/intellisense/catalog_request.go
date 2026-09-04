package intellisense

import (
	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/directive"
	httpversion "github.com/unkn0wn-root/resterm/internal/http/version"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
)

func addRequestArgs(c argumentCatalog) {
	c.add(args{named: authArgs}, directive.Auth)
	c.add(args{named: patchArgs}, directive.Patch)
	c.add(args{named: []argument{
		optValue("use", "Reference a named patch profile", namesValue(func(_ Context, sc Scope) ([]string, string) {
			return sc.Profiles.Patch, "apply profile"
		})).repeats(),
	}}, directive.Apply)
	c.add(args{named: bodyArgs, single: true}, directive.Body)
	c.add(args{named: settingArgs, single: true}, directive.Setting)
	c.add(args{named: settingArgs}, directive.Settings)
	c.add(
		args{value: directiveValue(filePath("query", "GraphQL query file", PathGraphQL, pathLoadOnly))},
		directive.Query,
	)
	c.add(
		args{value: directiveValue(filePath("variables", "GraphQL variables file", PathJSON, pathLoadOnly))},
		directive.Variables,
	)
	c.add(args{value: directiveValue(flag("enabled", "Toggle GraphQL")), single: true}, directive.GraphQL)
}

var authArgs = []argument{
	word("request", "Make the auth directive explicitly request-scoped").withExample("bearer {{token}}"),
	word("file", "Define auth inherited by later requests in this file").withExample("bearer {{token}}"),
	word("global", "Define auth inherited across the workspace").withExample("bearer {{token}}"),
	word("none", "Disable inherited auth for the current request"),
	word("basic", "Basic auth with username and password").withExample("user pass"),
	word("bearer", "Bearer token auth").withExample("{{token}}"),
	word("apikey", "API key auth in header or query").withExample("header X-API-Key {{key}}"),
	word("oauth2", "Built-in OAuth 2.0 token acquisition and caching").chains(),
	word("command", "Run a CLI command and inject its token output").
		withOption(`argv=["gh","auth","token"]`),
	word("header", "API key placement in headers"),
	word("query", "API key placement in query string"),
	opt("token_url", "OAuth2 token endpoint URL", "https://auth.example.com/oauth/token"),
	opt("auth_url", "OAuth2 authorization endpoint URL", "https://auth.example.com/authorize"),
	opt("client_id", "OAuth2 client ID", "{{clientId}}"),
	opt("client_secret", "OAuth2 client secret", "{{clientSecret}}"),
	choice(
		"grant",
		"OAuth2 grant type",
		oauth.GrantClientCredentials,
		oauth.GrantPassword,
		oauth.GrantAuthorizationCode,
	),
	opt("scope", "OAuth2 scope list", `"read write"`),
	opt("audience", "OAuth2 audience", "https://api.example.com"),
	opt("resource", "OAuth2 resource indicator", "https://graph.microsoft.com"),
	choice("client_auth", "OAuth2 client credential transport", oauth.ClientAuthBasic, oauth.ClientAuthBody),
	opt("username", "Password grant username", "{{user.email}}"),
	opt("password", "Password grant password", "{{user.password}}"),
	opt("cache_key", "Reuse cached auth state across requests", "myapi"),
	opt("redirect_uri", "OAuth2 redirect URI", "http://127.0.0.1:8484/callback"),
	opt("code_verifier", "PKCE code verifier", "{{pkce.verifier}}"),
	choice(
		"code_challenge_method",
		"PKCE challenge method",
		oauth.CodeChallengeS256,
		oauth.CodeChallengePlain,
	),
	opt("state", "OAuth2 state value", "{{oauth.state}}"),
	opt("header", "Override injected header name", "Authorization"),
	opt("argv", "Command argv as JSON array", `["gh","auth","token"]`),
	choice("format", "Command output format", string(authcmd.FormatText), string(authcmd.FormatJSON)),
	opt("scheme", "Command auth header scheme", "Bearer"),
	opt("token_path", "JSON path to token value", "access_token"),
	opt("type_path", "JSON path to token type", "token_type"),
	opt("expiry_path", "JSON path to absolute expiry", "expires_at"),
	opt("expires_in_path", "JSON path to relative expiry seconds", "expires_in"),
	opt("ttl", "Fallback command auth cache TTL", "10m"),
	opt("timeout", "Command auth timeout", "5s"),
}

var patchArgs = []argument{
	word("file", "Define a file-scoped reusable patch profile"),
	word("global", "Define a workspace-global reusable patch profile"),
}

var bodyArgs = []argument{
	toggle("expand", "Expand body templates").alias("expand-templates"),
	toggle("inline", "Force an inline body").alias("raw"),
}

var settingArgs = []argument{
	opt("base-url", "Base URL for relative HTTP requests", "https://api.example.com/v1/"),
	opt("timeout", "Request timeout (e.g. 5s)", "5s"),
	opt("proxy", "HTTP proxy URL", "http://proxy"),
	flag("followredirects", "Follow redirects"),
	flag("insecure", "Skip TLS verification (HTTP)"),
	flag("no-cookies", "Disable cookies for this request"),
	opt(
		"forward-credentials-on-redirect",
		"Origins a redirect may carry credentials to",
		"https://cdn.example.com",
	),
	opt("max-redirects", "Redirects to follow (count or none)", "20"),
	opt("max-response-size", "Response body limit (size or none)", "100mb"),
	opt("sse-max-line-bytes", "Default SSE line limit for every request", "4mb"),
	opt("sse-max-event-bytes", "Default SSE event limit for every request", "8mb"),
	opt("ws-max-message-bytes", "Default WebSocket message limit for every request", "32kb"),
	choice(
		"http-version",
		"HTTP protocol version",
		httpversion.Format(httpversion.V11),
		httpversion.Format(httpversion.V2),
	),
	flag("http-insecure", "Skip TLS verification (HTTP)"),
	filePath("http-root-cas", "Extra HTTP root CAs", PathAny, pathWord).list().alias("http-root-ca"),
	choice(
		"http-root-mode",
		"HTTP root CA mode",
		string(tlsconfig.RootModeAppend),
		string(tlsconfig.RootModeReplace),
	),
	filePath("http-client-cert", "HTTP client certificate", PathAny, pathWord),
	filePath("http-client-key", "HTTP client key", PathAny, pathWord),
	flag("grpc-insecure", "Skip TLS verification (gRPC)"),
	filePath("grpc-root-cas", "Extra gRPC root CAs", PathAny, pathWord).list().alias("grpc-root-ca"),
	choice(
		"grpc-root-mode",
		"gRPC root CA mode",
		string(tlsconfig.RootModeAppend),
		string(tlsconfig.RootModeReplace),
	),
	filePath("grpc-client-cert", "gRPC client certificate", PathAny, pathWord),
	filePath("grpc-client-key", "gRPC client key", PathAny, pathWord),
	opt("grpc-max-recv-size", "Max gRPC response size", "16MB"),
	opt("grpc-max-send-size", "Max gRPC request size", "16MB"),
	choice("grpc-compression", "gRPC request compression", grpcx.CompressionNames()...),
}
