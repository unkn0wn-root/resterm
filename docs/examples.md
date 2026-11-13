# Examples

The `_examples/` directory has request files that show Resterm's features. Run the commands below from the root of a clone of this repository. Resterm picks the `dev` environment from `resterm.env.json` unless you pass `--env`.

## Start here

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [basic.http](../_examples/basic.http) with [resterm.env.json](../_examples/resterm.env.json) | A file-wide base URL, GET and POST, a binary download, time and random data helpers, and bearer auth. Its `resterm.env.json` holds the environments that most files in this folder use. | runs as-is | `resterm run --all _examples/basic.http` | [Request file anatomy](request-files.md) |
| [nested.http](../_examples/nested.http) | File and request variables with nested environment values in a JSON body, with tags. | runs as-is | `resterm run --all _examples/nested.http` | [Variables and environments](variables.md) |
| [service-groups.http](../_examples/service-groups.http) | Requests grouped by tag, such as smoke checks and provisioning. | runs as-is | `resterm run --tag billing _examples/service-groups.http` | [Selecting what to run](cli/run.md#selecting-what-to-run) |
| [xml.http](../_examples/xml.http) | XML bodies and explicit inline angle-prefixed text. | runs as-is | `resterm run --all _examples/xml.http` | [Body content](request-files.md#body-content) |
| [transport.http](../_examples/transport.http) | Timeout, proxy, and `@no-log` samples. The timeout request fails on purpose, and the local mock stands in for the proxy. | start the local mock first | `resterm _examples/transport.http` | [HTTP transport and settings](http-settings.md) |

## Variables and auth

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [scopes.http](../_examples/scopes.http) | Global, file, and request variables and captures, checked with `@assert`. | runs as-is | `resterm run --all _examples/scopes.http` | [Captures](captures.md) |
| [auth_scopes.http](../_examples/auth_scopes.http) | Default auth inheritance across global, file and request scopes, plus `@auth none`. | runs as-is | `resterm run --all _examples/auth_scopes.http` | [Authentication directives](authentication.md#authentication-directives) |
| [oauth2.http](../_examples/oauth2.http) | Capturing a token by hand compared with the `@auth oauth2` directive. | needs your own server | `resterm _examples/oauth2.http` | [OAuth 2.0 directive](authentication.md#oauth-20-directive) |
| [auth_command.http](../_examples/auth_command.http) | Command-backed auth from `gh auth token`, with file defaults, named definitions with `use=`, `@apply` reuse, JSON output and custom headers. The `mycli` request needs a CLI of your own. | needs a GitHub token | `resterm _examples/auth_command.http` | [Command-backed auth](authentication.md#command-backed-auth) |
| [grouped/grouped-environments.http](../_examples/grouped/grouped-environments.http) with [mock.http](../_examples/grouped/mock.http) and [resterm.env.json](../_examples/grouped/resterm.env.json) | Grouped environments declared in their own `resterm.env.json`, with a mock backend that echoes what each request carried. | start the local mock first | `resterm _examples/grouped/grouped-environments.http` | [Grouped environments](variables.md#grouped-environments) |

## Scripts and checks

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [scripts.http](../_examples/scripts.http) | A RestermScript pre-request block that sets a token, headers and body, then an `@assert` check. | runs as-is | `resterm run --all _examples/scripts.http` | [Directives](rts/directives.md) |
| [rts/rts_all_features.http](../_examples/rts/rts_all_features.http) with [helpers.rts](../_examples/rts/helpers.rts), [loops.rts](../_examples/rts/loops.rts), [apply_patch.rts](../_examples/rts/apply_patch.rts), [pre_request.rts](../_examples/rts/pre_request.rts), [validators.rts](../_examples/rts/validators.rts), [users.rts](../_examples/rts/users.rts) and [users.json](../_examples/rts/users.json) | RestermScript expressions, modules, the standard library, `@when`, `@skip-if`, `@for-each`, and pre-request and test blocks. | runs as-is | `resterm run --all _examples/rts/rts_all_features.http` | [RestermScript](rts/README.md) |
| [rts/apply_profiles.http](../_examples/rts/apply_profiles.http) | Reusable `@patch` profiles applied with `@apply use=`. | runs as-is | `resterm run _examples/rts/apply_profiles.http` | [@patch](rts/directives.md#patch) |
| [javascript.http](../_examples/javascript.http) with [js/save-body.js](../_examples/js/save-body.js) | JavaScript pre-request and test scripts, and a test that saves a response body to a file. | runs as-is | `resterm run --all _examples/javascript.http` | [JavaScript hooks](scripting.md) |
| [trace.http](../_examples/trace.http) | One request within its trace budget and one that fails on purpose by breaching it. | runs as-is | `resterm _examples/trace.http` | [Timeline & tracing](ui-tour.md#timeline--tracing) |

## Protocols

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [graphql.http](../_examples/graphql.http) with [queries/workspace.graphql](../_examples/queries/workspace.graphql) and [queries/workspace.variables.json](../_examples/queries/workspace.variables.json) | Inline and file-based GraphQL requests, with queries and variables in `queries/`. | runs as-is | `resterm run --all _examples/graphql.http` | [GraphQL](graphql.md) |
| [sse_manual.http](../_examples/sse_manual.http) | An SSE feed to watch in the Stream tab. | runs as-is | `resterm _examples/sse_manual.http` | [WebSocket and SSE](streaming.md) |
| [sse_scripted.http](../_examples/sse_scripted.http) | An SSE feed checked by captures and `@assert`. | runs as-is | `resterm run _examples/sse_scripted.http` | [WebSocket and SSE](streaming.md) |
| [websocket_manual.http](../_examples/websocket_manual.http) | A WebSocket echo to drive from the live console. | runs as-is | `resterm _examples/websocket_manual.http` | [Stream tab, history, and console](streaming.md#stream-tab-history-and-console) |
| [websocket_scripted.http](../_examples/websocket_scripted.http) | A WebSocket echo driven by `@ws` steps and checked by `@assert` and an `@rts test` block. | runs as-is | `resterm run _examples/websocket_scripted.http` | [WebSocket and SSE](streaming.md) |
| [streaming.http](../_examples/streaming.http) | The `@sse` and `@websocket` directives side by side, against public test servers. | runs as-is | `resterm run --all _examples/streaming.http` | [WebSocket and SSE](streaming.md) |
| [grpc.http](../_examples/grpc.http) with [proto/analytics.proto](../_examples/proto/analytics.proto) | gRPC with server reflection, metadata and a plaintext override, and a call that reads a local `.proto` file. | needs your own server | `resterm run --request GrpcReportStatus _examples/grpc.http` | [gRPC](grpc.md) |

## Workflows and mocks

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [workflows.http](../_examples/workflows.http) | A full workflow with captures, overrides, and expectations. | runs as-is | `resterm run --workflow sample-order _examples/workflows.http` | [Workflows](workflows.md) |
| [compare.http](../_examples/compare.http) | `@compare` directives and multi-environment sweeps started from the CLI. | runs as-is | `resterm run --all --compare dev,stage,prod _examples/compare.http` | [Compare runs](compare-runs.md) |
| [mocks.http](../_examples/mocks.http) with [mock-user.json](../_examples/mock-user.json) | Mock routes, variants, matchers, selectors, latency, and fixture bodies. | start the local mock first | `resterm _examples/mocks.http` | [Mock servers](mock-servers.md) |
| [polling-retries.http](../_examples/polling-retries.http) | Polling and retries against mock sequences in the same file. Two requests fail on purpose. | start the local mock first | `resterm _examples/polling-retries.http` | [Polling and retries](polling-and-retries.md) |
| [recording.http](../_examples/recording.http) | A local walkthrough of `resterm record` against a mock server. | start the local mock first | `resterm mock _examples/recording.http --addr 127.0.0.1:9001` | [Recording traffic](recording.md) |

## Import and customize

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [curl-import.http](../_examples/curl-import.http) | Sample output from the CLI curl importer. The Run command imports the first of the two curl commands it was made from. | runs as-is | `resterm --from-curl "curl -G https://api.example.com/search -d q=hello -H 'X-Flag: yes'" --http-out curl.http` | [Import curl and OpenAPI](cli/import.md) |
| [openapi-spec.yml](../_examples/openapi-spec.yml) | An OpenAPI document to try the OpenAPI importer on. | runs as-is | `resterm --from-openapi _examples/openapi-spec.yml --http-out openapi.http` | [Import curl and OpenAPI](cli/import.md) |
| [themes/aurora.toml](../_examples/themes/aurora.toml) with [daybreak.toml](../_examples/themes/daybreak.toml) | The Aurora and Daybreak themes. | runs as-is | `RESTERM_THEMES_DIR="$(pwd)/_examples/themes" resterm` | [Testing a theme](theming.md#testing-a-theme) |
| [bindings/bindings.toml](../_examples/bindings/bindings.toml) | A sample `bindings.toml`. | runs as-is | `RESTERM_CONFIG_DIR="$(pwd)/_examples/bindings" resterm` | [Key bindings](key-bindings.md) |

## Your own servers

| Example | What it shows | Needs | Run | Docs |
| --- | --- | --- | --- | --- |
| [ssh.http](../_examples/ssh.http) | SSH jump profiles and per-request overrides. | needs your own SSH host | `resterm _examples/ssh.http` | [SSH tunnels](ssh-tunnels.md) |
| [k8s.http](../_examples/k8s.http) | Kubernetes profile scopes, non-pod targets, named ports, and gRPC over `@k8s`. | needs your own cluster | `resterm _examples/k8s.http` | [Kubernetes port-forwards](kubernetes.md) |
