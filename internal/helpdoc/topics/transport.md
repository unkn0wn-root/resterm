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

You can omit the scheme when a URL starts with a host and port. Resterm adds `http://`, so `GET localhost:8080/users` works. With `@websocket`, the same URL uses `ws://`. Bracketed IPv6 addresses, such as `[::1]`, also work without a port.

A hostname without a port remains a relative URL, so write `http://example.com/users` when `example.com` is the destination. Templates are expanded before the same rules are applied: `localhost:8080` and `http://example.com` work as host values, while a bare `example.com` needs `base-url`.

REST, GraphQL, SSE, and WebSocket share `base-url`; WebSockets map `http`/`https` to `ws`/`wss`. gRPC targets are unchanged. A request method is still required, for example `GET /health`.
