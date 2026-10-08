# HTTP transport and settings

- Set connection defaults on the command line with [shared flags](cli/README.md#shared-execution-flags) such as `--timeout`, `--follow`, `--insecure`, and `--proxy`.
- To change a setting for one request, use `@setting`, `@settings`, or `@timeout`:

  ```http
  ### Fast timeout
  # @name TimeoutDemo
  # @timeout 2s
  GET https://httpbin.org/delay/5
  ```

## Relative request URLs

Set `base-url` to resolve shorter HTTP request targets without repeating a host:

```http
# File default when declared before the first request
# @setting base-url https://api.example.com/v1/

### List users
GET users?page=2

### Root-relative health check
GET /health

### One-request override
# @setting base-url https://admin.example.com/api/
GET status
```

The same setting can be selected globally through an environment:

```json
{
  "dev": {
    "settings.base-url": "https://api.dev.example.com/v1/"
  },
  "prod": {
    "settings.base-url": "https://api.example.com/v1/"
  }
}
```

Request settings override file settings, which override environment settings: global < file < request. Setting keys are case-insensitive. The base URL and the request URL can contain templates, such as `# @setting base-url {{services.api.base}}`.

Resterm expands the base only when the final request URL is relative. An absolute URL ignores `base-url`, even if the base contains an invalid or unresolved non-empty value. Skipped requests and gRPC requests do not use it either.

Relative URLs follow the standard URI-reference rules. You can append a path, start from the host's root, replace the query, move up a directory, or name a different host:

| Base `https://api.example.com/v1/` | Effective URL |
| --- | --- |
| `users` | `https://api.example.com/v1/users` |
| `/users` | `https://api.example.com/users` |
| `../health` | `https://api.example.com/health` |
| `?page=2` | `https://api.example.com/v1/?page=2` |
| `//uploads.example.com/x` | `https://uploads.example.com/x` |
| `https://other.example.com/x` | unchanged |

The trailing slash matters: `https://api.example.com/v1/` plus `users` keeps `/v1/`, while `https://api.example.com/v1` plus `users` produces `https://api.example.com/users`. Resterm does not insert a slash.

The base must be an absolute `http` or `https` URL with a host. It may include a port and path, but cannot contain a username or password (userinfo), a query, or a fragment. An explicitly empty value is an error. A relative request without a usable base fails before connecting.

A network-path URL (`//host/path`) changes the destination host. The request's headers and configured authentication go to that host too.

REST, GraphQL, SSE, and WebSocket requests share the setting. For WebSockets, an effective `http` URL becomes `ws` and `https` becomes `wss`. WebSocket fragments are rejected. `base-url` does not change gRPC targets. A request method remains required, so `GET /users` is valid while a bare `/users` line is not a request.

## URLs without a scheme

You can omit `http://` when a request URL starts with a host and port. Resterm uses plain HTTP:

| Request line | Effective URL |
| --- | --- |
| `GET localhost:8080/users` | `http://localhost:8080/users` |
| `GET 127.0.0.1:8080/users` | `http://127.0.0.1:8080/users` |
| `GET [::1]:8080/users` | `http://[::1]:8080/users` |
| `GET [::1]/users` | `http://[::1]/users` |

The port distinguishes these URLs from relative paths. Bracketed IPv6 addresses are also clear without a port, so `[::1]/users` works. These URLs are absolute and ignore `base-url`. Resterm always adds `http://`. Write `https://` when you want TLS.

Only the start of the request URL is checked. A URL in a query value does not affect the destination: `GET localhost:8080/p?next=http://example.com` still connects to `localhost:8080`. A known scheme without `//`, such as `https:443/path`, is rejected instead of being treated as a host named `https`.

For a request with `# @websocket`, Resterm changes the added scheme to `ws://`. For example, `GET localhost:8080/socket` connects to `ws://localhost:8080/socket`. The `@websocket` directive starts the session. Using `WS` as the method does not.

A hostname without a port remains a relative URL. With `base-url` set to `https://api.example.com/v1/`, `GET example.com/users` resolves to `https://api.example.com/v1/example.com/users`. Write `http://example.com/users` when `example.com` is the destination host.

Templates are expanded before these rules are applied. When a variable represents a server, include either a port or a scheme:

| Value of `{{host}}` | Result of `GET {{host}}/users` |
| --- | --- |
| `localhost:8080` | `http://localhost:8080/users` |
| `127.0.0.1:9000` | `http://127.0.0.1:9000/users` |
| `http://example.com` | `http://example.com/users` |
| `example.com` | A relative URL that needs `base-url` |

A template can also provide part of the URL. Both `GET http://{{host}}/users` and `GET localhost:{{port}}/users` work.

## Other HTTP settings

- HTTP version: `@setting http-version 1.1` (accepts `1.1`, `2`, `HTTP/1.1`, `HTTP/2`). A trailing `HTTP/1.1` on the request line also sets the version. Explicit settings win. `2` is strict and fails if the response is not HTTP/2. WebSocket requests are incompatible with `2`.
- HTTP/1.0 is not supported. Resterm rejects `http-version 1.0`, trailing `HTTP/1.0`, and other unsupported version tokens such as `HTTP/3`.
- Only a trailing `HTTP/<major>` or `HTTP/<major>.<minor>` is read as a version. Any other trailing text stays part of the URL, so `GET https://example.com/a http/foo` requests `/a%20http/foo`.
- Set a `Host` header to choose a virtual host. The connection and TLS server name still use the URL's host. Cookies use the `Host` value. A redirect to a path keeps the `Host` header. A redirect that names a host uses that host instead. WebSocket handshakes send the `Host` header too.
- Resterm removes credentials before following a redirect to another origin or to another `Host`, even on the same address. An origin is the scheme, host, and port. Changing any of these creates a different origin. Resterm removes known credential headers and any custom header named by `@auth`, even when that header was already on the request.
- Use `@setting forward-credentials-on-redirect` to send credentials to specific origins:

  ```http
  # @setting forward-credentials-on-redirect https://cdn.example.com https://media.example.com
  ```

  Matches are exact. A listed origin does not include its subdomains or other ports. `wss://` matches the same origin as `https://`, and `ws://` matches the same origin as `http://`. The default is `false`. Use `true` to send credentials to any origin. A list is safer because it only allows the named origins. An empty value is an error.

  These rules cannot be changed:

  - If a redirect chain moves from HTTPS to HTTP, Resterm stops sending credentials for the rest of the chain. This also applies if a later redirect returns to HTTPS or to the original origin. The HTTP server controls every redirect that follows.
  - A `Cookie` header is never copied to another origin. The cookie jar may still add cookies that belong to the new origin.
  - OAuth token requests stay on the token endpoint's origin. A 307 or 308 redirect can resend the client secret in the body, so removing headers would not protect it.
  - When a redirect goes to another origin, Resterm sends only the origin of the previous URL in the `Referer` header. It removes the path and query, so a key added with `@auth ... query` does not reach the new origin. Resterm checks each redirect separately. If the next URL has the same origin, the `Referer` keeps the full previous URL, even if an earlier redirect crossed an origin boundary. Resterm leaves an explicitly set `Referer` unchanged.

- Resterm follows up to 10 redirects by default. Set another limit with `@setting max-redirects 20` or `--max-redirects`. Use `0` or `none` to stop at the first redirect. `@setting followredirects false` also stops at the first redirect. There is no unlimited setting because the request must stop if the server sends a redirect loop.
- Response bodies are limited to 32 MiB. Change the limit with `@setting max-response-size 100mb`, `@setting max-response-size none`, or `--max-response-size`. Resterm checks the size after decompressing the body. A larger body stops the request with an error.
- SSE and WebSocket size limits, and the `sse-max-line-bytes`, `sse-max-event-bytes`, and `ws-max-message-bytes` settings, are covered in [WebSocket and SSE](streaming.md#limits).
- Requests use an in-memory cookie jar per environment. Cookies are isolated between environments, and `@setting no-cookies true` disables cookies for a request without clearing the stored jar. Use `Ctrl+Shift+G` (or `g Shift+G`) to clear cookies for the current environment.
- TLS per request: `# @settings http-root-cas=a.pem http-client-cert=cert.pem http-client-key=key.pem http-insecure=true` for a single line, or `@setting key value` per line (`http-root-cas` accepts lists separated by spaces, commas, or semicolons, and paths are relative). GraphQL, REST, WebSocket, and SSE requests all share these HTTP settings.
- Custom root CAs replace system roots by default (strict). Set `http-root-mode append` or `grpc-root-mode append` if you want to keep system roots in addition to your own.
- File-level defaults: place `# @setting key value` or `# @settings key1=val1 ...` before the first request to apply them to every request in that file. Request-level overrides still win.
- HTTP, transport, and TLS settings include `base-url`, `http-*`, `grpc-*`, `timeout`, `proxy`, `followredirects`, `max-redirects`, `forward-credentials-on-redirect`, `max-response-size`, `sse-max-line-bytes`, `sse-max-event-bytes`, `ws-max-message-bytes`, `ws-compression`, `insecure`, and `no-cookies`. Resterm ignores unknown keys.
- Boolean settings (`followredirects`, `insecure`, `no-cookies`, `ws-compression`, `http-insecure`, `grpc-insecure`) accept `true`/`false`, `yes`/`no`, `on`/`off`, and `1`/`0`. A key written on its own is a flag meaning `true`, so `# @setting insecure`, `# @settings insecure`, and `# @setting insecure true` are the same thing. `@setting` also accepts the `key=value` form, so `# @setting insecure=false` means what `# @settings insecure=false` does.
- Settings validate their values. A value the setting does not accept fails the request instead of falling back to a default, so a typo cannot silently leave TLS verification or redirects at the wrong setting. This covers booleans, `timeout` (a Go duration such as `30s`), `proxy` (a URL with a scheme and host, such as `http://host:8080`), `http-version`, and `http-root-mode`/`grpc-root-mode`. A non-empty `base-url` is resolved and validated only when a relative HTTP-family target needs it. Writing a key with an empty value (`# @settings insecure=`, or `"settings.insecure": ""` in an environment file) is reported as a missing value rather than treated as a flag.
- Environment defaults: `resterm.env.json` can carry global settings under the `settings.` prefix (for example, `"settings.base-url": "https://api.example.com/v1/"`, `"settings.http-root-cas": "ca-dev.pem"`, `"settings.grpc-insecure": "false"`). Precedence is global (env) < file < request.
- OAuth token exchanges reuse the same HTTP TLS settings (root CAs, client cert/key, `http-insecure`) as the main request.
