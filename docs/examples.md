# Examples

The `_examples/` directory has request files you can run right away:

- `basic.http` - simple GET/POST with bearer auth.
- `nested.http` - requests with nested JSON bodies and tags.
- `service-groups.http` - requests grouped by tag, such as smoke checks and provisioning.
- `scopes.http` - global, file, and request captures.
- `scripts.http` - pre-request and test scripting patterns.
- `xml.http` - XML bodies and explicit inline angle-prefixed text.
- `transport.http` - timeout, proxy, and `@no-log` samples.
- `auth_scopes.http` - default auth inheritance across global/file/request scopes plus `@auth none`.
- `auth_command.http` - command-backed auth with file-scoped defaults, named definitions with `use=`, `gh auth token`, patch-based reuse, JSON output parsing, and custom header examples.
- `oauth2.http` - capturing a token by hand compared with the `@auth oauth2` directive.
- `graphql.http` - inline and file-based GraphQL requests, with queries and variables in `queries/`.
- `grpc.http` - gRPC reflection and descriptor usage, with a descriptor set in `descriptors/`.
- `streaming.http` - an SSE feed and a WebSocket chat in one file.
- `sse_manual.http` and `sse_scripted.http` - an SSE feed to watch in the Stream tab, and one checked by captures and a test script.
- `websocket_manual.http` and `websocket_scripted.http` - a WebSocket echo to drive from the console, and one driven by `@ws` steps and checked by a test script.
- `workflows.http` - a full workflow with captures, overrides, and expectations.
- `polling-retries.http` - polling and retries against mock sequences in the same file.
- `compare.http` - `@compare` directives and multi-environment sweeps started from the CLI.
- `trace.http` - one request within its trace budget and one that breaches it.
- `mocks.http` - mock routes, variants, matchers, selectors, latency, and fixture bodies, such as `mock-user.json`.
- `recording.http` - a local walkthrough of `resterm record` against a mock server.
- `ssh.http` - SSH jump profiles and per-request overrides.
- `k8s.http` - Kubernetes profile scopes, non-pod targets, named ports, and gRPC over `@k8s`.
- `curl-import.http` - sample output from the CLI curl importer.
- `openapi-spec.yml` - an OpenAPI document to try the OpenAPI importer on.
- `resterm.env.json` - the environments the examples above use.

Folders:

- `grouped/` - grouped environments, with a mock backend that echoes what each request carried.
- `rts/` - RestermScript modules, `@apply` profiles, loops, pre-request blocks, and a file that uses every feature.
- `bindings/` - a sample `bindings.toml`. See [Key bindings](key-bindings.md).
- `themes/` - the Aurora and Daybreak themes. See [Theming](theming.md).

Open one in Resterm, pick the right environment from `resterm.env.json`, and send the requests to see how each feature works.
