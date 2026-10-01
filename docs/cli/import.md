# Import curl and OpenAPI

Convert curl into Resterm request files:

```bash
resterm --from-curl "curl https://example.com -H 'X-Test: 1'" --http-out example.http
resterm --from-curl ./requests.curl --http-out requests.http
cat requests.curl | resterm --from-curl - --http-out requests.http
```

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

Mock generation emits every concrete OpenAPI response status and media example. Named examples become named scenarios, and when a response has no example Resterm samples its schema. Range responses such as `2XX` and `default` are skipped. External examples and binary example bodies cannot produce a deterministic inline mock, so they are dropped with a diagnostic.
