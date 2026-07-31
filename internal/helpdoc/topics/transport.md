# HTTP Transport and Settings

Use `@timeout`, `@setting`, and `@settings` to control transport behavior for a request or file.

```http
# @timeout 5s
# @setting followredirects false
# @settings http-insecure=true no-cookies
GET https://example.com/slow
```

Settings cover TLS, proxies, redirects, cookies, HTTP versions, and certificate roots. Place them before the first request to apply to the whole file. Request settings win over file and environment defaults.
