# Import curl and OpenAPI

Convert curl into Resterm request files:

```bash
resterm --from-curl "curl https://example.com -H 'X-Test: 1'" --http-out example.http
resterm --from-curl ./requests.curl --http-out requests.http
cat requests.curl | resterm --from-curl - --http-out requests.http
```

One file or paste can hold several curl commands. Every line that starts with `curl` begins a new one, and each command becomes its own request. `--next` inside a command also starts a new request.

Most curl flags have a direct equivalent:

| curl | Resterm |
| --- | --- |
| `-X`, `-H`, `--url`, `-I` | Method, headers, and URL. |
| `-d`, `--data-raw`, `--data-binary`, `--data-urlencode`, `--json`, `-F`, `--form-string` | Request body. |
| `-G` | Moves the data into the query string and sends a GET. |
| `-T <file>` | Sends the file as the body. The method becomes PUT unless `-X` sets one. |
| `-u user:pass` | `@auth basic`. |
| `-A`, `-e`, `-b` | `User-Agent`, `Referer`, and `Cookie` headers. |
| `--compressed` | An `Accept-Encoding` header. |
| `-k`, `-x`, `-L`, `-m` | The `http-insecure`, `proxy`, `followredirects`, and `timeout` settings. |
| `--cacert`, `--cert`, `--key` | The `http-root-cas`, `http-client-cert`, and `http-client-key` settings. |

Other flags, such as `--retry`, `--connect-timeout`, `--max-redirs`, `-v`, `-o`, and `--http2`, are ignored. The generated file starts with a comment that names the Resterm version, then the original command under `Source:`, then a warning for each ignored flag.

Generate a collection from OpenAPI:

```bash
resterm \
  --from-openapi openapi.yml \
  --http-out openapi.http \
  --openapi-mode both \
  --openapi-resolve-refs \
  --openapi-server-index 1
```

`--from-openapi` also takes an `http(s)` URL and fetches the spec directly:

```bash
resterm --from-openapi https://petstore3.swagger.io/api/v3/openapi.json --http-out petstore.http
```

For a URL spec, relative `servers` URLs are resolved against it, and `--openapi-resolve-refs`
follows external `$ref`s over HTTP. `--insecure` and `--proxy` apply to the fetch.

The base URL comes from the first entry in the spec's `servers` list and is saved as a file variable named `baseUrl`. Pass `--openapi-server-index` to pick another entry, counting from 0, and `--openapi-base-var` to use another variable name. An index past the end of the list falls back to the first server.

Security schemes in the spec become `@auth` on the requests that use them:

| Scheme | Generated auth | Placeholder globals |
| --- | --- | --- |
| HTTP basic | `@auth basic` | `auth.username`, `auth.password` |
| HTTP bearer | `@auth bearer` | `auth.token` |
| API key | `@auth apikey` in the header or query parameter the spec names. The header defaults to `X-API-Key`. A key sent as a cookie goes into the request's `Cookie` header instead, because `@auth apikey` has no cookie placement. | `auth.apiKey` |
| OAuth 2.0 | `@auth oauth2` with the client credentials, password, or authorization code flow, picked in that order | `oauth.clientId`, `oauth.clientSecret`, plus `oauth.scope`, `oauth.username`, and `oauth.password` when the flow needs them |
| OAuth 2.0 implicit flow only | `@auth bearer` | `auth.token` |

The globals are written at the top of the file with placeholder values such as `replace-with-token`, and secrets use `@global-secret`. Replace the placeholders before you send requests. Schemes Resterm cannot convert are skipped with a warning.

Mock generation creates a mock for every concrete response status and media example in the spec. Named examples become named scenarios, and when a response has no example Resterm samples its schema. Range responses such as `2XX` and `default` are skipped. External examples and binary example bodies cannot produce a deterministic inline mock, so they are dropped with a diagnostic.

Without `--http-out`, Resterm picks the output name:

- a curl file such as `requests.curl` becomes `requests.http` next to it
- an inline curl command or one read from stdin becomes `curl.http` in the current directory
- an OpenAPI file such as `openapi.yml` becomes `openapi.http` next to it
- an OpenAPI URL is named after the last part of its path and written to the current directory, so `https://example.com/v3/spec.json` becomes `spec.http`

An existing output file is replaced without asking. `--from-curl` and `--from-openapi` cannot be used together.
