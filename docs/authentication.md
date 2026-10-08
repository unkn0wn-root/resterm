# Authentication

## Static tokens

Use `@auth bearer {{token}}` or an `Authorization: Bearer {{token}}` header. Combine it with `@global` or environment values to reuse the token.

To use the same authentication for every request in a file, add a file-scoped definition:

```http
# @auth file bearer {{auth.token}}

### Inherits file auth
GET {{base.url}}/profile

### Opt out for one request
# @auth none
GET {{base.url}}/public
```

## Captured tokens

Capture a value at runtime and reuse it in later requests:

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

Resterm fetches the token for you, caches it per environment, refreshes it when it expires, and adds the `Authorization: Bearer ...` header to the request. Three grant types are supported.

### Client credentials grant

Use this for machine-to-machine calls where no user is involved.

```http
### Service-to-service call
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{svc.clientId}} client_secret={{svc.clientSecret}} scope="api:read api:write"
GET https://api.example.com/internal/status
```

By default, the client ID and secret are sent to the token endpoint with HTTP Basic authentication. Some providers want them as form fields instead. Use `client_auth=body` for that:

```http
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="{{oauth.scope}}" client_auth=body
GET {{base.url}}/resource
```

Resterm puts the client ID and secret into the Basic header as written. The OAuth 2.0 specification asks clients to URL-encode both values first, and some providers expect this. With those providers, a secret containing `+` or `%`, or a client ID containing `:`, is rejected. Wrap the values in `url.encode`:

```http
# @const oauth.id {{= url.encode(env.get("oauth.clientId")) }}
# @const oauth.secret {{= url.encode(env.get("oauth.clientSecret")) }}

### Resource
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.id}} client_secret={{oauth.secret}}
GET {{base.url}}/resource
```

If your provider accepts form fields, `client_auth=body` works as well. Do not combine it with `url.encode`. The form body is already encoded, so the secret would be encoded twice.

### Password grant

For older systems that log in with a username and password.

```http
### Resource owner password
# @auth oauth2 token_url=https://auth.example.com/oauth/token client_id={{app.clientId}} client_secret={{app.clientSecret}} grant=password username={{user.email}} password={{user.password}} scope="profile"
GET https://api.example.com/me
```

### Authorization code + PKCE

With `grant=authorization_code`, Resterm runs the whole flow for you:

1. Opens your system browser at `auth_url` with the authorization request.
2. Starts a temporary HTTP server on localhost to catch the redirect.
3. Exchanges the authorization code for tokens at `token_url` and sends the PKCE verifier with it.
4. Caches the token and adds it to your request.

#### Redirect URI behavior

The `redirect_uri` controls where the authorization server sends the user after login:

| Configuration | Result |
| --- | --- |
| Omit `redirect_uri` | `http://127.0.0.1:<random-port>/oauth/callback` |
| `redirect_uri=http://127.0.0.1:8080/callback` | Uses port 8080 with path `/callback` |
| `redirect_uri=http://localhost:0/auth` | Random port, custom path `/auth` |

The redirect URI has a few rules:

- It must use `http://`, not `https://`. See [RFC 8252](https://datatracker.ietf.org/doc/html/rfc8252).
- The host must be `127.0.0.1` or `localhost`. Other hosts are rejected.
- Register the redirect URI pattern with your OAuth provider. Most allow `http://127.0.0.1:*` or similar.

#### PKCE details

PKCE (Proof Key for Code Exchange) helps protect your login if someone intercepts the authorization code. Resterm generates the values needed for the exchange:

- **code_verifier** - 64 random bytes, base64url-encoded (about 86 characters). You can set your own. It must be 43 to 128 characters long (RFC 7636).
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

The authorization code flow has a 2-minute timeout by default, which gives you time to log in from the browser. If you need longer, set `@timeout` on the request. Resterm uses it when it is longer than 2 minutes.

### Custom token header

Some APIs expect tokens in a non-standard header. Use the `header` parameter to change where the token goes:

```http
### API expecting X-Access-Token header
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} header=X-Access-Token
GET https://api.example.com/data
```

When `header` is something other than `Authorization`, Resterm sends only the raw token, without the `Bearer ` prefix. With the default `Authorization` header, the value is `Bearer <token>`.

## Digest auth

Use `# @auth digest <username> <password>` for HTTP Digest authentication (RFC 7616).

```http
### Digest-protected endpoint
# @auth digest {{api.user}} {{api.password}}
GET {{base.url}}/reports
```

Resterm sends the request without a Digest header. If the server returns a supported `401` challenge in `WWW-Authenticate`, Resterm retries once with an `Authorization: Digest` header and returns that response.

- Supported algorithms are `MD5`, `SHA-256` and `SHA-512-256`, including their `-sess` variants. Resterm uses the first supported challenge in the server's list.
- Resterm prefers `qop=auth`. With `qop=auth-int`, it also hashes the request body. Without `qop`, it uses the older RFC 2069 format. Session algorithms (`-sess`) require `qop`.
- Cookies set with the challenge are saved in the cookie jar and sent on the retry.
- Each redirect can get its own Digest header under the same credential rules as Basic auth. Sending credentials to another origin requires [`forward-credentials-on-redirect`](http-settings.md#other-http-settings). Credentials are never sent after an HTTPS-to-HTTP redirect, even if a later redirect returns to HTTPS.
- An explicit `Authorization` header takes precedence over `@auth` and is sent unchanged. Otherwise, `@auth digest` overrides any username and password in the URL.

Digest auth works with HTTP and SSE requests. Resterm does not support Digest auth for WebSocket or gRPC requests. An explicit `Authorization` header still takes precedence over `@auth` for these requests.

If you used `# @auth Digest <value>` to send a custom header, replace it with `Digest: <value>` in the request. `digest` is now a reserved auth type: one value is rejected, and additional values are read as a username and password.

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

Older files share a token through `cache_key`. The first request runs the command and fills the cache; later requests reuse it by naming the key. This still works, but the first request must run before the others:

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
| Basic | `# @auth basic user pass` | Adds `Authorization: Basic …`. Templates expand inside parameters. |
| Bearer | `# @auth bearer {{token}}` | Adds `Authorization: Bearer …`. |
| Digest | `# @auth digest user pass` | Retries once after the server's `401` Digest challenge. See [Digest auth](#digest-auth). |
| API key | `# @auth apikey header X-API-Key {{key}}` | `api-key` works too. Write the placement, the name, and the value. `placement` can be `header` or `query`. An `auth` dict in `@apply` or `@patch` may leave out `name`, which then defaults to the `X-API-Key` header. |
| Custom header | `# @auth Authorization CustomValue` | Any header and value. For a header named after an auth type, such as `Digest`, use a normal request header. |
| Command | `# @auth command cmd="gh auth token"` | Runs a non-interactive command without a shell, reads its `stdout` output, and adds an auth header before sending the request. |
| Named command | `# @auth use=gh` | Uses a command auth defined once with `@auth file` or `@auth global` and a name. |
| OAuth 2.0 | `# @auth oauth2 token_url=... client_id=...` | Fetches and caches tokens. Supports client_credentials, password, and authorization_code with PKCE. |

Scopes:

- Bare `@auth ...` is request-scoped.
- `@auth request ...` is an explicit request-scoped form.
- `@auth file ...` defines inherited auth for later requests in the same document.
- `@auth global ...` defines inherited auth for the whole workspace. File-scoped auth wins when both exist.
- `@auth file command name=<name> ...` and `@auth global command name=<name> ...` define a named command auth. A named definition needs `cmd` or `argv` and is not inherited. Requests pick it with `@auth use=<name>`.
- `@auth none` disables inherited auth for the current request.
- An `@auth` line with an error is not skipped. A request that would take its auth from that line (directly, through inheritance, or through `use=`) fails instead of going out with other auth or none.

### OAuth 2.0 parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `token_url` | Yes | - | Token endpoint URL. Must be provided at least once per `cache_key`. |
| `auth_url` | For auth code | - | Authorization endpoint. Required when `grant=authorization_code`. |
| `client_id` | Yes | - | Your application's client ID. |
| `client_secret` | No | - | Client secret (omit for public clients using PKCE). |
| `grant` | No | `client_credentials` | Grant type: `client_credentials`, `password`, or `authorization_code`. |
| `scope` | No | - | Space-separated scopes to request. |
| `audience` | No | - | Target API audience (for example, Auth0). |
| `resource` | No | - | Resource indicator (for example, Azure AD). |
| `client_auth` | No | `basic` | How to send credentials: `basic` (Authorization header) or `body` (form fields). Falls back to `body` automatically for public clients. |
| `header` | No | `Authorization` | Which header receives the token. Use this when an API expects tokens in a custom header like `X-Access-Token`. |
| `username` | For password | - | Resource owner username (only for `grant=password`). |
| `password` | For password | - | Resource owner password (only for `grant=password`). |
| `cache_key` | No | auto | Sets the cache key yourself. Useful when several requests should share one token even if their parameters differ a little. When omitted, Resterm builds the key from the token URL, client ID, scope, and other fields. |
| `redirect_uri` | No | auto | Callback URL for authorization code flow. See [Redirect URI behavior](#redirect-uri-behavior). |
| `code_verifier` | No | auto | PKCE verifier (43-128 characters per RFC 7636). Auto-generated when omitted. |
| `code_challenge_method` | No | `s256` | PKCE method: `s256` (recommended) or `plain`. |
| `state` | No | auto | CSRF protection token. Auto-generated when omitted. |

Any other `key=value` pairs are sent as extra form parameters to both the authorization and token endpoints.

### How token caching works

Resterm caches tokens per environment and `cache_key`. When a request needs a token:

1. If a valid cached token exists (not expired, with a 30-second safety margin), Resterm reuses it.
2. If the cached token is expired and has a `refresh_token`, Resterm tries to refresh it.
3. If the refresh fails or there is no token, Resterm fetches a new one from the token endpoint.

For grouped environments, "per environment" means the full group selection. Changing just the credentials profile gives you a separate OAuth and command-auth cache. An explicit `cache_key` is also tied to that selection.

Older tokens saved under an explicit key do not identify which selection they belong to. Resterm ignores those tokens and fetches new ones for the current selection.

You can define the full OAuth parameters once, then use only `cache_key` in later requests:

```http
### First request - seeds the cache
# @auth oauth2 token_url={{oauth.tokenUrl}} client_id={{oauth.clientId}} client_secret={{oauth.clientSecret}} scope="read write" cache_key=myapi
GET {{base.url}}/users

### Later request - reuses cached token
# @auth oauth2 cache_key=myapi
GET {{base.url}}/projects
```

If you leave out `token_url` on a later directive and the cache has not been seeded yet, Resterm fails with `@auth oauth2 requires token_url (include it once per cache_key to seed the cache)`.

### Command auth parameters

| Parameter | Required | Default | Description |
| --- | --- | --- | --- |
| `name` | No | - | Names a `file` or `global` definition so requests can pick it with `use=`. A named definition is not inherited. A request line cannot take a name. |
| `cmd` | One of `cmd` or `argv` | - | Command line, split into arguments like a shell would split it, but no shell runs. See [Command lines](#command-lines). |
| `argv` | One of `cmd` or `argv` | - | JSON array of command arguments. Bare JSON works, for example `argv=["gh","auth","token"]`. Outer single quotes also work and are useful when you want to keep whitespace exactly as written, for example `argv='["gh", "auth", "token"]'`. |
| `format` | No | `text` | Parse `stdout` as `text` or `json`. |
| `header` | No | `Authorization` | Target header name. |
| `scheme` | No | auto | Prefix for the final header value. When omitted, `Authorization` gets `Bearer` and custom headers get the raw token. |
| `token_path` | For `format=json` | - | JSON path to the token value. Uses the same JSON path behavior as RTS captures and helpers. |
| `type_path` | No | - | JSON path to the token type, used for `Authorization` headers when `scheme` is omitted. |
| `expiry_path` | No | - | JSON path to an absolute expiry value (RFC3339, RFC3339Nano, Unix seconds, or Unix milliseconds). |
| `expires_in_path` | No | - | JSON path to a relative lifetime in seconds. |
| `ttl` | No | - | How long a cached token stays valid when the command output has no expiry fields. Needs a named definition or `cache_key`. |
| `cache_key` | No | - | Names a command-auth slot shared by every directive that uses the same key. Seed it once with the full command, then reuse it with `cache_key` only. Named definitions are simpler for new files. |
| `timeout` | No | request timeout | Per-command timeout, bounded by the request timeout. |

`@auth command` accepts only the options in this table, and any other option is an error. The same rule applies to `{auth: {type: "command", ...}}` in `@apply` and `@patch`. `@auth use=<name>` accepts only `header`, `scheme`, and `timeout`. Everything else belongs to the definition.

### Command lines

`cmd` splits its value into arguments and runs the first one directly. No shell runs, so pipes, redirects, wildcard patterns (globbing), `$VAR`, and `$(...)` are passed to the command as plain text.

- Spaces and tabs separate arguments.
- Single quotes keep everything inside them as written.
- Double quotes group words and accept `\"` and `\\`.
- Outside quotes, a backslash escapes a space, a quote, or another backslash. Any other backslash stays, so `C:\tools\gh.exe` works unquoted.
- A `{{...}}` template stays in one argument even when it contains spaces. Templates expand after the split, so a value with spaces never becomes two arguments.

Wrap `cmd` in the kind of quote the command line does not use: `cmd="gcloud auth print-access-token --account 'me@example.com'"` or `cmd='mycli --name "Ada Lovelace"'`. Without quotes, `cmd=gh auth token` is an error. It does not run `gh` alone. `cmd=mycli --role=admin` is an error too, because `--role=admin` reads as an unknown option. So is a quote left open.

### Command auth behavior

- Commands run during auth preparation, before the request is sent.
- Resterm runs `@auth command` without a shell. Shell front ends such as `sh`, `bash`, `zsh`, `cmd`, and `pwsh` are rejected.
- The command inherits Resterm's environment and runs from the directory of the file that defines it, so a global definition behaves the same from every request file.
- Interactive login flows are not supported. Authenticate the CLI outside Resterm first, then use its non-interactive token-printing command.
- A named definition caches its token for the session, per environment and workspace. `ttl`, `expiry_path`, or `expires_in_path` make it expire sooner. Editing the definition runs the command again. `resterm run --persist-auth` keeps the cache between runs.
- A `use=` name that no file or global definition has fails the request with the line that named it.
- An unnamed definition without `cache_key` runs the command for every request.
- With `cache_key`, the first full directive seeds a reusable command-auth config for that environment. Later directives can use only `cache_key`. Empty fields take their values from the seeded config. If the slot has not been seeded yet, Resterm fails with `@auth command requires cmd or argv (include it once per cache_key to seed the cache)`.
- Reusing a `cache_key` with different settings for obtaining the token fails right away. These settings include `cmd` or `argv`, `format`, JSON paths, `ttl`, and related source fields. You can still change `header`, `scheme`, and `timeout` per request.
- Explain preview never runs commands. It only injects a header when a valid cached result already exists, so name the definition or add `cache_key` to preview it.
- Text mode accepts exactly one non-empty line from `stdout`. Multi-line output fails. Use `format=json` for that.
- Successful command output is treated as secret. Resterm redacts the raw token and the final injected header value in the explain and history views.

Some APIs take the token with another scheme. The Amazon ECR registry API takes the `get-authorization-token` value as Basic auth, valid for 12 hours:

```http
# @auth file command name=ecr cmd="aws ecr get-authorization-token --region {{aws.region}} --output text --query authorizationData[].authorizationToken" scheme=Basic ttl=11h

### Image tags
# @auth use=ecr
GET https://{{aws.account}}.dkr.ecr.{{aws.region}}.amazonaws.com/v2/{{repo}}/tags/list
```

`aws ecr get-login-password` prints the decoded registry password for `docker login --username AWS`. It is not a header value, so use `get-authorization-token` for HTTP requests.
