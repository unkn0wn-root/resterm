# HTTP Transport and Settings

Use `@timeout`, `@setting`, and `@settings` to control transport behavior for a request or file.

```http
# @timeout 5s
# @setting base-url https://api.example.com/v1/
# @setting followredirects false
# @settings http-insecure=true no-cookies
GET slow
```

Settings cover base URLs, TLS, proxies, redirects, cookies, HTTP versions, and certificate roots. Place them before the first request to apply to the whole file. Request settings win over file and environment defaults.

`base-url` accepts an absolute `http` or `https` URL with an optional port and path. Relative targets use standard URL rules: `users` joins the base path, `/users` replaces it, and `//other.example/users` may change the host. Keep a trailing slash when the last base path segment is a directory. Absolute request URLs ignore the setting.

REST, GraphQL, SSE, and WebSocket share `base-url`; WebSockets map `http`/`https` to `ws`/`wss`. gRPC targets are unchanged. A request method is still required, for example `GET /health`.
