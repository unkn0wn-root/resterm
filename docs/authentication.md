# Authentication

## Static tokens

Use `@auth bearer {{token}}` or `Authorization: Bearer {{token}}` headers. Combine with `@global` or environment values for reuse.

When you want one auth definition to apply to many requests, scope it:

```http
# @auth file bearer {{auth.token}}

### Inherits file auth
GET {{base.url}}/profile

### Opt out for one request
# @auth none
GET {{base.url}}/public
```

## Captured tokens

Capture values at runtime and reuse them in subsequent requests:

```http
### Login
# @capture global-secret auth.token = response.json.token
POST {{base.url}}/login

{
  "user": "{{user.email}}",
  "password": "{{user.password}}"
}

### Authorized request
# @auth bearer {{auth.token}}
GET {{base.url}}/profile
```

## OAuth 2.0 directive

Resterm handles the full OAuth 2.0 token lifecycle: fetching tokens, caching them per environment, refreshing when expired, and injecting the `Authorization: Bearer ...` header automatically. Three grant types are supported.

### Client credentials grant

Best for machine-to-machine authentication where no user is involved.

```http
### Service-to-service call
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{svc.clientId}} client_secret={{svc.clientSecret}} scope="api:read api:write"
GET https://api.example.com/internal/status
```

By default, credentials are sent via HTTP Basic authentication. Use `client_auth=body` to send them as form fields instead (required by some providers):

```http
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="{{oauth.scope}}" client_auth=body
GET {{base.url}}/resource
```

### Password grant

For legacy systems that require username/password authentication.

```http
### Resource owner password
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{app.clientId}} client_secret={{app.clientSecret}} grant=password username={{user.email}} password={{user.password}} scope="profile"
GET https://api.example.com/me
```

### Authorization code + PKCE

When you use `grant=authorization_code`, Resterm handles the entire OAuth automatically:

1. **Browser launch** - Opens your system browser to `auth_url` with the authorization request.
2. **Local callback server** - Spins up a temporary HTTP server on localhost to capture the redirect.
3. **Code exchange** - Exchanges the authorization code for tokens at `token_url`, including the PKCE verifier.
4. **Token injection** - Caches the token and injects it into your request.

#### Redirect URI behavior

The `redirect_uri` controls where the authorization server sends the user after login:

| Configuration | Result |
| --- | --- |
| Omit `redirect_uri` | `http://127.0.0.1:<random-port>/oauth/callback` |
| `redirect_uri=http://127.0.0.1:8080/callback` | Uses port 8080 with path `/callback` |
| `redirect_uri=http://localhost:0/auth` | Random port, custom path `/auth` |

**Constraints:**
- Must use `http://` scheme (not `https://`) - [RFC 8252](https://datatracker.ietf.org/doc/html/rfc8252)
- Host must be `127.0.0.1` or `localhost` - external hosts are rejected
- Register the redirect URI pattern with your OAuth provider (most allow `http://127.0.0.1:*` or similar)

#### PKCE details

PKCE (Proof Key for Code Exchange) protects against authorization code interception. Resterm generates these automatically:

- **code_verifier** - 64 random bytes, base64url-encoded (~86 characters). You can provide your own if needed (must be 43-128 characters per RFC 7636).
- **code_challenge** - SHA-256 hash of the verifier, base64url-encoded.
- **state** - 24 random bytes for CSRF protection.

#### Example: Public client with PKCE

```http
### GitHub OAuth (public client, no secret)
# @auth oauth2 auth_url=https://github.com/login/oauth/authorize token_url=https://github.com/login/oauth/access_token client_id={{github.clientId}} scope="repo read:user" grant=authorization_code
GET https://api.github.com/user
Accept: application/json
```

#### Example: Confidential client

```http
### Auth0 with client secret
# @auth oauth2 auth_url=https://{{auth0.domain}}/authorize token_url=https://{{auth0.domain}}/oauth/token client_id={{auth0.clientId}} client_secret={{auth0.clientSecret}} scope="openid profile" audience={{auth0.audience}} grant=authorization_code
GET {{api.url}}/userinfo
```

#### Timeout behavior

Authorization code flow has a 2-minute timeout by default (to give users time to complete login in the browser). If you need longer, the request's `@timeout` setting is respected as long as it exceeds 2 minutes.

### Custom token header

Some APIs expect tokens in a non-standard header. Use the `header` parameter to change where the token goes:

```http
### API expecting X-Access-Token header
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} header=X-Access-Token
GET https://api.example.com/data
```

When `header` is set to something other than `Authorization`, Resterm injects just the raw token (without the "Bearer " prefix). When using the default `Authorization` header, the full `Bearer <token>` format is used.

## Command-backed auth

If the token is already in an environment variable, you do not need a command. Read it once per file:

```http
# @file token env:GH_TOKEN
# @auth file bearer {{token}}

### User
GET https://api.github.com/user
```

Use `@auth command` when a CLI you already have can print the token, for example `gh auth token`. To apply it to every request in a file, define it with file scope:

```http
# @auth file command cmd="gh auth token"

### User
GET https://api.github.com/user

### Repos
GET https://api.github.com/user/repos

### Public endpoint without inherited auth
# @auth none
GET https://api.github.com/rate_limit
```

This runs the command for every request. To run it once and reuse the token, give the definition a name and pick it per request with `use=`. A global definition works from every file in the workspace:

```http
# @auth global command name=gh cmd="gh auth token"

### User
# @auth use=gh
GET https://api.github.com/user

### Same token in a custom header
# @auth use=gh header=X-GitHub-Token
GET https://api.github.com/rate_limit
```

Any request can run first. The token stays cached for the session in each environment, and `ttl` refreshes tokens that expire:

```http
# @auth file command name=gcloud cmd="gcloud auth print-access-token" ttl=50m
```

Structured output works too:

```http
### Internal CLI with JSON output
# @auth command cmd="mycli auth print --json" format=json token_path=access_token type_path=token_type expires_in_path=expires_in cache_key=myapi
GET https://api.example.com/projects
```

Older files share a token through `cache_key`. The first request seeds the slot and later requests name only the key. This still works, but the first request has to run before the others:

```http
### Seed a reusable command-auth slot
# @auth command argv=["gh","auth","token"] cache_key=github-cli
GET https://api.github.com/user

### Reuse it later with cache_key only
# @auth command cache_key=github-cli
GET https://api.github.com/user/repos
```

## Authentication directives

| Type | Syntax | Notes |
| --- | --- | --- |
| Basic | `# @auth basic user pass` | Injects `Authorization: Basic …`. Templates expand inside parameters. |
| Bearer | `# @auth bearer {{token}}` | Injects `Authorization: Bearer …`. |
| API key | `# @auth apikey header X-API-Key {{key}}` | `placement` can be `header` or `query`. Defaults to `X-API-Key` header if name omitted. |
| Custom header | `# @auth Authorization CustomValue` | Arbitrary header/value pair. |
| Command | `# @auth command cmd="gh auth token"` | Runs a non-interactive command without a shell, parses `stdout`, and injects a header during auth preparation. |
| Named command | `# @auth use=gh` | Uses a command auth defined once with `@auth file` or `@auth global` and a name. |
| OAuth 2.0 | `# @auth oauth2 token_url=... client_id=...` | Built-in token acquisition and caching (client_credentials/password/authorization_code + PKCE). |

Scopes:

- Bare `@auth ...` is request-scoped.
- `@auth request ...` is an explicit request-scoped form.
- `@auth file ...` defines inherited auth for later requests in the same document.
- `@auth global ...` defines workspace-global inherited auth; file-scoped auth wins when both exist.
- `@auth file command name=<name> ...` and `@auth global command name=<name> ...` define a named command auth. A named definition needs `cmd` or `argv` and is not inherited. Requests pick it with `@auth use=<name>`.
- `@auth none` disables inherited auth for the current request.
- An `@auth` line with an error is not skipped. A request that would take its auth from that line, directly, through inheritance, or through `use=`, fails instead of going out with other auth or none.

### OAuth 2.0 parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `token_url` | Yes | - | Token endpoint URL. Must be provided at least once per `cache_key`. |
| `auth_url` | For auth code | - | Authorization endpoint. Required when `grant=authorization_code`. |
| `client_id` | Yes | - | Your application's client ID. |
| `client_secret` | No | - | Client secret (omit for public clients using PKCE). |
| `grant` | No | `client_credentials` | Grant type: `client_credentials`, `password`, or `authorization_code`. |
| `scope` | No | - | Space-separated scopes to request. |
| `audience` | No | - | Target API audience (Auth0, etc.). |
| `resource` | No | - | Resource indicator (Azure AD, etc.). |
| `client_auth` | No | `basic` | How to send credentials: `basic` (Authorization header) or `body` (form fields). Falls back to `body` automatically for public clients. |
| `header` | No | `Authorization` | Which header receives the token. Use this when an API expects tokens in a custom header like `X-Access-Token`. |
| `username` | For password | - | Resource owner username (only for `grant=password`). |
| `password` | For password | - | Resource owner password (only for `grant=password`). |
| `cache_key` | No | auto | Override the cache identity. Useful when multiple requests should share the same token even if their parameters differ slightly. When omitted, Resterm derives the key from token URL, client ID, scope, and other fields. |
| `redirect_uri` | No | auto | Callback URL for authorization code flow. See [Redirect URI behavior](#redirect-uri-behavior). |
| `code_verifier` | No | auto | PKCE verifier (43-128 characters per RFC 7636). Auto-generated when omitted. |
| `code_challenge_method` | No | `s256` | PKCE method: `s256` (recommended) or `plain`. |
| `state` | No | auto | CSRF protection token. Auto-generated when omitted. |

Any additional `key=value` pairs are forwarded as extra form parameters to both the authorization and token endpoints.

### How token caching works

Resterm caches tokens per environment and `cache_key`. When a request needs a token:

1. If a valid cached token exists (not expired, with 30-second safety margin), it's reused immediately.
2. If the cached token has a `refresh_token` and is expired, Resterm attempts a refresh.
3. If refresh fails or no token exists, a fresh token is fetched from the token endpoint.

For grouped environments, "per environment" means the complete group selection. Changing only the credentials profile uses a separate OAuth and command-auth cache, and explicit `cache_key` values are scoped to that selection too. Tokens persisted under an explicit key before this scoping existed cannot be attributed to a selection, so Resterm ignores them and fetches a fresh token under the new scoped key.

This means you can define full OAuth parameters once, then reference just `cache_key` in subsequent requests:

```http
### First request - seeds the cache
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="read write" cache_key=myapi
GET {{base.url}}/users

### Later request - reuses cached token
# @auth oauth2 cache_key=myapi
GET {{base.url}}/projects
```

If you skip `token_url` on a follow-up directive and the cache hasn’t been seeded yet, Resterm will error with `@auth oauth2 requires token_url (include it once per cache_key to seed the cache)`.

### Command auth parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | No | - | Names a `file` or `global` definition so requests can pick it with `use=`. A named definition is not inherited. A request line cannot take a name. |
| `cmd` | One of `cmd` or `argv` | - | Command line, split into arguments like a shell would split it, but no shell runs. See [Command lines](#command-lines). |
| `argv` | One of `cmd` or `argv` | - | JSON array of command arguments. Bare JSON works, for example `argv=["gh","auth","token"]`. Outer single quotes are also accepted and are useful when you want to preserve whitespace exactly, for example `argv='["gh", "auth", "token"]'`. |
| `format` | No | `text` | Parse `stdout` as `text` or `json`. |
| `header` | No | `Authorization` | Target header name. |
| `scheme` | No | auto | Explicit prefix for the final header value. When omitted, `Authorization` defaults to `Bearer`, while custom headers get the raw token. |
| `token_path` | For `format=json` | - | JSON path to the token value. Uses the same JSON path behavior as RTS captures and helpers. |
| `type_path` | No | - | Optional token type for `Authorization` headers when `scheme` is omitted. |
| `expiry_path` | No | - | Optional absolute expiry value (RFC3339, RFC3339Nano, Unix seconds, or Unix milliseconds). |
| `expires_in_path` | No | - | Optional relative lifetime in seconds. |
| `ttl` | No | - | How long a cached token stays valid when the command output has no expiry fields. Needs a named definition or `cache_key`. |
| `cache_key` | No | - | Names a command-auth slot shared by every directive that uses the same key. Seed it once with the full command, then reuse it with `cache_key` only. Named definitions are simpler for new files. |
| `timeout` | No | request timeout | Per-command timeout, bounded by the request timeout. |

`@auth command` accepts only the options in this table, and any other option is an error. The same rule applies to `{auth: {type: "command", ...}}` in `@apply` and `@patch`. `@auth use=<name>` accepts only `header`, `scheme`, and `timeout`. Everything else belongs to the definition.

### Command lines

`cmd` splits its value into arguments and runs the first one directly. Nothing goes through a shell, so pipes, redirects, globbing, `$VAR`, and `$(...)` are passed to the command as plain text.

- Spaces and tabs separate arguments.
- Single quotes keep everything inside them as written.
- Double quotes group words and accept `\"` and `\\`.
- Outside quotes, a backslash escapes a space, a quote, or another backslash. Any other backslash stays, so `C:\tools\gh.exe` works unquoted.
- A `{{...}}` template stays in one argument even when it contains spaces. Templates expand after the split, so a value with spaces never becomes two arguments.

Wrap `cmd` in the quote kind the command line does not use: `cmd="gcloud auth print-access-token --account 'me@example.com'"` or `cmd='mycli --name "Ada Lovelace"'`. Without quotes, `cmd=gh auth token` is an error rather than a run of `gh` alone. So is `cmd=mycli --role=admin`, where `--role=admin` would read as an unknown option, and so is a quote left open.

### Command auth behavior

- Commands run during auth preparation, before the request is sent.
- Resterm runs `@auth command` without a shell. Shell front ends such as `sh`, `bash`, `zsh`, `cmd`, and `pwsh` are rejected.
- The command inherits Resterm's environment and runs from the directory of the file that defines it, so a global definition behaves the same from every request file.
- Interactive login flows are not supported. Authenticate the CLI outside Resterm first, then use its non-interactive token-printing command.
- A named definition caches its token for the session, per environment and workspace. `ttl`, `expiry_path`, or `expires_in_path` make it expire sooner. Editing the definition runs the command again. `resterm run --persist-auth` keeps the cache between runs.
- A `use=` name that no file or global definition has fails the request with the line that named it.
- An unnamed definition without `cache_key` runs the command for every request.
- With `cache_key`, the first full directive seeds a reusable command-auth config for that environment. Later directives can use only `cache_key`; empty fields inherit from the seeded config. If the slot has not been seeded yet, Resterm errors with `@auth command requires cmd or argv (include it once per cache_key to seed the cache)`.
- Reusing the same `cache_key` with different acquisition settings (`cmd` or `argv`, `format`, JSON paths, `ttl`, and related source fields) fails fast. `header`, `scheme`, and `timeout` can still vary per request.
- Explain preview never executes commands. It only injects a header when a valid cached result already exists, so name the definition or add `cache_key` to preview it.
- Text mode accepts exactly one non-empty line from `stdout`. Multi-line output fails with an error and should be switched to `format=json`.
- Successful command output is treated as secret. Resterm redacts the raw token and the final injected header value in explain/history views.

Some APIs take the token with another scheme. The Amazon ECR registry API takes the `get-authorization-token` value as Basic auth, valid for 12 hours:

```http
# @auth file command name=ecr cmd="aws ecr get-authorization-token --region {{aws.region}} --output text --query authorizationData[].authorizationToken" scheme=Basic ttl=11h

### Image tags
# @auth use=ecr
GET https://{{aws.account}}.dkr.ecr.{{aws.region}}.amazonaws.com/v2/{{repo}}/tags/list
```

`aws ecr get-login-password` prints the decoded registry password for `docker login --username AWS`. It is not a header value, so use `get-authorization-token` for HTTP requests.
