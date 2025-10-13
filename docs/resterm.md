# Resterm Documentation

## Installation

### Prebuilt binaries

1. Download the archive for your platform from the [GitHub Releases](https://github.com/unkn0wn-root/resterm/releases) page (macOS, Linux, or Windows; amd64 and arm64 builds are published).
2. Mark the binary as executable (`chmod +x resterm` on Unix), then copy it into a directory on your `PATH`.
3. Launch with `resterm --help` to confirm the CLI is available.

### Build from source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

This requires Go 1.24 or newer. The binary will be installed in `$(go env GOPATH)/bin`.

---

## Quick Start

1. Place one or more `.http` or `.rest` files in a working directory (or use the samples under `_examples/`).
2. Run `resterm --workspace path/to/project`.
3. Use the file pane to select a request file, then highlight a request and press `Ctrl+Enter` to send it.
4. Inspect responses in the Pretty, Raw, Headers, Diff, or History tabs on the right.

A minimal `.http` file looks like this:

```http
### Fetch Status
# @name health
GET https://httpbin.org/status/204
User-Agent: resterm
Accept: application/json

### Create Resource
# @name create
POST https://httpbin.org/anything
Content-Type: application/json

{
  "id": "{{$uuid}}",
  "note": "created from Resterm"
}
```

---

## UI Tour

### Layout

- **Sidebar**: upper half lists `.http`/`.rest` files (filtered by workspace root); lower half lists requests discovered in the active document.
- **Editor**: middle pane with modal editing (view mode by default, `i` to insert, `Esc` to return to view). Inline syntax highlighting marks metadata, headers, and bodies.
- **Response panes**: right-hand side displays the most recent response, with optional splits for side-by-side comparisons.
- **Header bar**: shows workspace, active environment, current request, and test summaries.
- **Command bar & status**: contextual hints, progress animations, and notifications.

### Core shortcuts

| Action | Shortcut |
| --- | --- |
| Send active request | `Ctrl+Enter` |
| Toggle help overlay | `?` |
| Toggle editor insert mode | `i` / `Esc` |
| Cycle focus (files -> requests -> editor -> response) | `Tab` / `Shift+Tab` |
| Focus requests / editor / response panes | `g+r` / `g+i` / `g+p` |
| Adjust editor/response width | `g+h` / `g+l` |
| Adjust files/requests split | `g+j` / `g+k` |
| Adjust requests/workflows split | `g+J` / `g+K` |
| File list incremental search | Start typing while focus is in the list |
| Open environment selector | `Ctrl+E` |
| Save file | `Ctrl+S` |
| Open file picker | `Ctrl+O` |
| New scratch buffer | `Ctrl+T` |
| Reparse current document | `Ctrl+P` (also `Ctrl+Alt+P`) |
| Refresh workspace files | `Ctrl+Shift+O` |
| Split response vertically / horizontally | `Ctrl+V` / `Ctrl+U` |
| Pin or unpin response pane | `Ctrl+Shift+V` |
| Choose target pane for next response | `Ctrl+F`, then arrow keys or `h` / `l` |
| Show globals summary / clear globals | `Ctrl+G` / `Ctrl+Shift+G` |
| Quit | `Ctrl+Q` (or `Ctrl+D`) |

The editor supports familiar Vim motions (`h`, `j`, `k`, `l`, `w`, `b`, `gg`, `G`, etc.), visual selections with `v` / `V`, yank and delete operations, undo/redo (`u` / `Ctrl+r`), and a search palette (`Shift+F`, toggle regex with `Ctrl+R` and `n` moves cursor forward and `p` backwards).

### Response panes

- **Pretty**: formatted JSON (or best-effort formatting for other types).
- **Raw**: exact payload text.
- **Headers**: request and response headers.
- **Stats**: latency summaries and histograms from `@profile` runs.
- **Diff**: compare the focused pane against the other response pane.
- **History**: chronological responses for the selected request (live updates). Open a full JSON preview with `p` or delete the focused entry with `d`.

Use `Ctrl+V` or `Ctrl+U` to split the response pane. The secondary pane can be pinned so subsequent calls populate only the primary pane, making comparisons easy.

### History and globals

- The history pane persists responses along with their request and environment metadata. Entries survive restarts (stored under the config directory; see [Configuration](#configuration)).
- `Ctrl+G` shows current globals (request/file/runtime) with secrets masked. `Ctrl+Shift+G` clears them for the active environment.
- `Ctrl+E` opens the environment picker to switch between `resterm.env.json` (or `rest-client.env.json`) entries.

---

## Workspaces & Files

- Resterm scans the workspace root for `.http` and `.rest` files. Use `--workspace` to set the root or rely on the directory of the file passed via `--file`. Add `--recursive` to traverse subdirectories (hidden directories are skipped).
- The file list supports incremental filtering (`/` is not required—just type while focused).
- The request list updates immediately when a file is saved or reparsed.
- Create a scratch buffer with `Ctrl+T` for ad-hoc experiments. These buffers are not written to disk unless you save them explicitly.

### Inline requests

You can execute simple requests without a `.http` file:

1. Type `GET https://api.example.com/users` (or just the URL) in the editor.
2. Place the cursor on the line and press `Ctrl+Enter`.

Inline requests support full URLs and a limited curl import:

```bash
curl \
  -X POST https://api.example.com/login \
  -H "Content-Type: application/json" \
  -d '{"user":"demo","password":"pass"}'
```

Resterm recognizes common curl flags (`-X`, `--request`, `-H`, `--header`, `-d/--data*`, `--json`, `--url`, `-u/--user`, `--head`, `--compressed`, `-F/--form`) and converts them into a structured request. Multiline commands joined with backslashes are supported.

---

## Variables and Environments

### Environment files

Resterm automatically searches, in order:

1. The directory of the opened file.
2. The workspace root.
3. The current working directory.

It loads the first `resterm.env.json` or `rest-client.env.json` it finds. The JSON can contain nested objects and arrays—they are flattened using dot and bracket notation (`services.api.base`, `plans.addons[0]`).

Example environment (`_examples/resterm.env.json`):

```json
{
  "dev": {
    "services": {
      "api": {
        "base": "https://httpbin.org/anything/api"
      }
    },
    "auth": {
      "token": "dev-token-123"
    }
  }
}
```

Switch environments with `Ctrl+E`. If multiple environments exist, Resterm defaults to `dev`, `default`, or `local` when available.

### Variable resolution order

When expanding `{{variable}}` templates, Resterm looks in:

1. Values set by scripts for the current execution (`vars.set` in pre-request or test scripts).
2. *Request-scope* variables (`@var request`, `@capture request`).
3. *Runtime globals* stored via captures or scripts (per environment).
4. *Document globals* (`@global`, `@var global`).
5. *File scope* declarations and `@capture file` values.
6. Selected environment JSON.
7. OS environment variables (case-sensitive with an uppercase fallback).

Dynamic helpers are also available: `{{$uuid}}`, `{{$timestamp}}` (Unix), `{{$timestampISO8601}}`, and `{{$randomInt}}`.

---

## Request File Anatomy

### Separators and comments

- Begin each request with a line that starts with `###`. Everything up to the next separator belongs to the same request.
- Lines prefixed with `#`, `//`, or `--` are treated as comments. Metadata directives live inside these comment blocks.

### Metadata directives

| Directive | Syntax | Description |
| --- | --- | --- |
| `@name` | `# @name identifier` | Friendly name used in the request list and history. |
| `@description` / `@desc` | `# @description ...` | Multi-line description (lines concatenate with newline). |
| `@tag` / `@tags` | `# @tag smoke billing` | Tags for grouping and filters (comma- or space-separated). |
| `@no-log` | `# @no-log` | Prevents the response body snippet from being stored in history. |
| `@log-sensitive-headers` | `# @log-sensitive-headers [true|false]` | Allow allowlisted sensitive headers (Authorization, Proxy-Authorization, API-token headers such as `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, etc.) to appear in history; omit or set to `false` to keep them masked (default). |
| `@setting` | `# @setting key value` | Per-request transport overrides (`timeout`, `proxy`, `followredirects`, `insecure`). |
| `@timeout` | `# @timeout 5s` | Equivalent to `@setting timeout 5s`. |

### Transport settings example

```http
### Fast timeout
# @name TimeoutDemo
# @timeout 2s
GET https://httpbin.org/delay/5
```

### Variable declarations

`@var` and `@global` provide static values evaluated before the request is sent.

| Scope | Syntax | Visibility |
| --- | --- | --- |
| Global | `# @global api.token value` or `# @var global api.token value` | Visible to every request and every file (per environment). |
| File | `# @var file upload.root https://storage.example.com` | Visible to all requests in the same document only. |
| Request | `# @var request trace.id {{$uuid}}` | Visible only to the current request (useful for tests). |

You can also use shorthand assignments outside comment blocks: `@requestId = {{$uuid}}`. Before the request line, these default to file scope; after the request line but before headers, they default to request scope.

Append `-secret` (`global-secret`, `file-secret`, `request-secret`) to mask stored values in summaries.

### Captures

`@capture <scope> <name> <expression>` evaluates after the response arrives and stores the result for reuse.

Expressions can reference:

- `{{response.status}}`, `{{response.statuscode}}`
- `{{response.body}}`
- `{{response.headers.<Header-Name>}}`
- `{{response.json.path}}` (dot/bracket navigation into JSON)
- Any template variables resolvable by the current stack

Example:

```http
### Seed session
# @name AnalyticsSeedSession
# @capture global-secret analytics.sessionToken {{response.json.json.sessionToken}}
# @capture file analytics.lastJobId {{response.json.json.jobId}}
# @capture request analytics.trace {{response.json.headers.X-Amzn-Trace-Id}}
POST https://httpbin.org/anything/analytics/sessions
```

### Body content

- **Inline**: everything after the blank line separating headers and body.
- **External file**: `< ./payloads/create-user.json` loads the file relative to the request file.
- **Inline includes**: lines in the body starting with `@ path/to/file` are replaced with the file contents (useful for multi-part templates).
- **GraphQL**: handled separately (see [GraphQL](#graphql)).

### Profiling requests

Add `# @profile` to any request to run it repeatedly and collect latency statistics without leaving the terminal. Profile runs are recorded in history with aggregated results; hit `p` on the entry to inspect the stored JSON.

```
### Benchmark health check
# @profile count=50 warmup=5 delay=100ms
GET https://httpbin.org/status/200
```

Flags:

- `count` - number of measured runs (defaults to 10).
- `warmup` - optional warmup runs that are executed but excluded from stats.
- `delay` - optional delay between runs (e.g. `250ms`).

When profiling completes the response pane's **Stats** tab shows percentiles, histograms, success/failure counts, and any errors that occurred.

### Workflows (multi-step workflows)

Group existing requests into repeatable workflows using `@workflow` blocks. Each step references a request by name and can override variables or expectations.

```
### Provision account
# @workflow provision-account on-failure=continue
# @step Authenticate using=AuthLogin expect.statuscode=200
# @step CreateProfile using=CreateUser vars.request.name={{vars.workflow.userName}}
# @step FetchProfile using=GetUser

### AuthLogin
POST https://example.com/auth

### CreateUser
POST https://example.com/users

### GetUser
GET https://example.com/users/{{vars.workflow.userId}}
```

Workflows parsed from the current document appear in the **Workflows** list on the left. Select one and press `Enter` (or `Space`) to run it. Resterm executes each step in order, respects `on-failure=continue`, and streams progress in the status bar. When the run completes the **Stats** tab shows a workflow summary, and a consolidated entry is written to history so you can review results later.

Key directives and tokens:

- `@workflow <name>` starts a workflow. Add `on-failure=<stop|continue>` to change the default behaviour and attach other tokens (e.g. `region=us-east-1`) which are surfaced under `Workflow.Options` for tooling.
- `@description` / `@tag` lines inside the workflow build the description and tag list shown in the UI and stored in history.
- `@step <optional-alias>` defines an execution step. Supply `using=<RequestName>` (required), `on-failure=<...>` for per-step overrides, `expect.status` / `expect.statuscode`, and any number of `vars.*` assignments.
- `vars.request.*` keys add step-scoped values that are available as `{{vars.request.<name>}}` during that request. They do not rewrite existing `@var` declarations automatically, so reference the namespaced token (or copy it in a pre-request script) when you want the override.
- `vars.workflow.*` keys persist between steps and are available anywhere in the workflow as `{{vars.workflow.<name>}}`, letting later requests reuse or mutate shared context (e.g. `vars.workflow.userId`).
- Unknown tokens on `@workflow` or `@step` are preserved in `Options`, allowing custom scripts or future features to consume them without changing the file format.
- `expect.status` supports quoted or escaped values, so you can write `expect.status="201 Created"` alongside `expect.statuscode=201`.

> **Tip:** Workflow assignments are expanded once when the request executes. If you need helpers such as `{{$uuid}}`, place them directly in the request/template or compute them via a pre-request script before assigning the value.
> **Tip:** Options are parsed like CLI flags; wrap values in quotes or escape spaces (`\ `) to keep text together (e.g. `expect.status="201 Created"`).

Every workflow run is persisted alongside regular requests in History; the newest entry is highlighted automatically so you can open the generated `@workflow` definition and results from the History pane immediately after the run.

### Authentication directives

| Type | Syntax | Notes |
| --- | --- | --- |
| Basic | `# @auth basic user pass` | Injects `Authorization: Basic …`. Templates expand inside parameters. |
| Bearer | `# @auth bearer {{token}}` | Injects `Authorization: Bearer …`. |
| API key | `# @auth apikey header X-API-Key {{key}}` | `placement` can be `header` or `query`. Defaults to `X-API-Key` header if name omitted. |
| Custom header | `# @auth Authorization CustomValue` | Arbitrary header/value pair. |
| OAuth 2.0 | `# @auth oauth2 token_url=... client_id=...` | Built-in token acquisition and caching (details below). |

#### OAuth 2.0 parameters

- `token_url` (required)
- `client_id`, `client_secret`
- `scope`, `audience`, `resource`
- `grant` (defaults to `client_credentials`; `password` and other grants supported when paired with `username`/`password`)
- `client_auth` (`basic` or `body`; default `basic`)
- `username`, `password` (for password credentials)
- `cache_key` (override cache identity; otherwise derived from token URL, client ID, scope, etc.)
- Additional `key=value` pairs are forwarded as extra form parameters.

Tokens are cached per environment and refreshed automatically if `refresh_token` or `expires_in` is returned.

### Scripting (`@script`)

Add `# @script pre-request` or `# @script test` followed by lines that start with `>`.

```http
# @script pre-request
> var token = vars.global.get("reporting.token") || `script-${Date.now()}`;
> vars.global.set("reporting.token", token, {secret: true});
> request.setHeader("Authorization", `Bearer ${token}`);
> request.setBody(JSON.stringify({ scope: "reports" }, null, 2));
```

Reference external scripts with `> < ./scripts/pre.js`.

See [Scripting API](#scripting-api) for available helpers.

---

## GraphQL

Enable GraphQL handling with `# @graphql` (requests start with it disabled). Resterm packages GraphQL requests according to HTTP method:

- **POST**: body becomes `{ "query": ..., "variables": ..., "operationName": ... }`.
- **GET**: query parameters `query`, `variables`, `operationName` are attached.

Available directives:

| Directive | Description |
| --- | --- |
| `@graphql [true|false]` | Enable/disable GraphQL processing for the request. |
| `@operation` / `@graphql-operation` | Sets the `operationName`. |
| `@variables` | Starts a variables block; inline JSON or `< file.json`. |
| `@query` | Loads the query from a file instead of the inline body. |

Example:

```http
### Inline GraphQL Query
# @graphql
# @operation FetchWorkspace
POST {{graphql.endpoint}}

query FetchWorkspace($id: ID!) {
  workspace(id: $id) {
    id
    name
  }
}

# @variables
{
  "id": "{{graphql.workspaceId}}"
}
```

---

## gRPC

gRPC requests start with a line such as `GRPC host:port`. Metadata directives describe the method and transport options.

| Directive | Description |
| --- | --- |
| `@grpc package.Service/Method` | Fully qualified method to call. |
| `@grpc-descriptor path/to/file.protoset` | Use a compiled descriptor set instead of server reflection. |
| `@grpc-reflection [true|false]` | Toggle server reflection (default `true`). |
| `@grpc-plaintext [true|false]` | Force plaintext or TLS. |
| `@grpc-authority value` | Override the HTTP/2 `:authority` header. |
| `@grpc-metadata key: value` | Add metadata pairs (repeatable). |

The request body contains protobuf JSON. Use `< payload.json` to load from disk. Responses display message JSON, headers, and trailers; history stores method, status, and timing alongside HTTP calls.

Example:

```http
### Generate Report Over gRPC
# @grpc analytics.ReportingService/GenerateReport
# @grpc-reflection true
# @grpc-plaintext true
# @grpc-authority analytics.dev.local
# @grpc-metadata x-trace-id: {{$uuid}}
GRPC {{grpc.host}}

{
  "tenantId": "{{tenant.id}}",
  "reportId": "rep-{{$uuid}}"
}
```

---

## Scripting API

Scripts run in an ES5.1-compatible Goja VM.

### Pre-request scripts (`@script pre-request`)

Objects:

- `request`
  - `getURL()`, `setURL(url)`
  - `getMethod()`, `setMethod(method)`
  - `getHeader(name)`, `setHeader(name, value)`, `addHeader(name, value)`, `removeHeader(name)`
  - `setBody(text)`
  - `setQueryParam(name, value)`
- `vars`
  - `get(name)`, `set(name, value)`, `has(name)`
  - `global.get(name)`, `global.set(name, value, options)`, `global.has(name)`, `global.delete(name)` (`options.secret` masks values)
- `console.log/warn/error` (no-op placeholders for compatibility)

Return values from `set*` helpers are ignored; side effects apply to the outgoing request.

### Test scripts (`@script test`)

Objects:

- `client.test(name, fn)` – registers a named test. Exceptions or manual failures mark the test as failed.
- `tests.assert(condition, message)` – add a pass/fail entry.
- `tests.fail(message)` – explicit failure.
- `response`
  - `status`, `statusCode`, `url`, `duration`
  - `body()` (raw string)
  - `json()` (parsed JSON or `null`)
  - `headers.get(name)`, `headers.has(name)`, `headers.all` (lowercase map)
- `vars` – same API as pre-request scripts (allows reading request/file/global values and writing request-scope values for assertions).
- `vars.global` – identical to pre-request usage; changes persist after the script.
- `console.*` – same placeholders as above.

Example test block:

```http
# @script test
> client.test("captures token", function () {
>   var token = vars.get("oauth.manualToken");
>   tests.assert(!!token, "token should be available");
> });
```

---

## Authentication

### Static tokens

Use `@auth bearer {{token}}` or `Authorization: Bearer {{token}}` headers. Combine with `@global` or environment values for reuse.

### Captured tokens

Capture values at runtime and reuse them in subsequent requests:

```http
### Login
# @capture global-secret auth.token {{response.json.token}}
POST {{base.url}}/login

{
  "user": "{{user.email}}",
  "password": "{{user.password}}"
}

### Authorized request
# @auth bearer {{auth.token}}
GET {{base.url}}/profile
```

### OAuth 2.0 directive

Resterm orchestrates the token request, caches the result per environment, and injects `Authorization: Bearer ...` headers automatically. Use `cache_key` when multiple requests with identical parameters should share the token.

```http
# @auth oauth2 token_url={{oauth.tokenUrl}} \
             client_id={{oauth.clientId}} \
             client_secret={{oauth.clientSecret}} \
             scope="{{oauth.scope}}" \
             audience={{oauth.audience}} \
             cache_key={{oauth.cacheKey}} \
             client_auth=body
```

---

## HTTP Transport & Settings

- Global defaults are passed via CLI flags (`--timeout`, `--follow`, `--insecure`, `--proxy`).
- Per-request overrides use `@setting` or `@timeout`.
- Requests inherit a shared cookie jar; cookies persist across sessions.
- Use `@no-log` to omit sensitive bodies from history snapshots.
- History is stored in `${RESTERM_CONFIG_DIR}/history.json` (defaults to the platform config directory) and retains up to ~500 entries. Set `RESTERM_CONFIG_DIR` to relocate it.

Body helpers:

- `< path` loads file contents as the body.
- `@ path` inside the body injects file contents inline.
- GraphQL payloads are normalized automatically.

---

## Response History & Diffing

- Every successful request produces a history entry with request text, method, status, duration, and a body snippet (unless `@no-log` is set). Values injected from `-secret` captures and allowlisted sensitive headers (Authorization, Proxy-Authorization, `X-API-Key`, `X-Access-Token`, `X-Auth-Key`, `X-Amz-Security-Token`, etc.) are masked automatically unless you opt-in with `@log-sensitive-headers`.
- History entries are environment-aware; selecting another environment filters the list automatically.
- When focused on the history list, press `Enter` to load a request into the editor without executing it. Use `r`/`Ctrl+R` (or your normal send shortcut such as `Ctrl+Enter` / `Cmd+Enter`) to replay the loaded entry.
- The Diff tab compares focused versus pinned panes, making regression analysis straightforward.

---

## CLI Reference

Run `resterm --help` for the latest list. Core flags:

| Flag | Description |
| --- | --- |
| `--file <path>` | Open a specific `.http`/`.rest` file on launch. |
| `--workspace <dir>` | Workspace root used for file discovery. |
| `--recursive` | Recursively scan the workspace for request files. |
| `--env <name>` | Select environment explicitly. |
| `--env-file <path>` | Provide an explicit environment JSON file. |
| `--timeout <duration>` | Default HTTP timeout (per request). |
| `--insecure` | Skip TLS certificate verification globally. |
| `--follow` | Control redirect following (default on; pass `--follow=false` to disable). |
| `--proxy <url>` | HTTP proxy URL. |


---

## Configuration

- Config directory: `$HOME/Library/Application Support/resterm` (macOS), `%APPDATA%\resterm` (Windows), or `$HOME/.config/resterm` (Linux/Unix). Override with `RESTERM_CONFIG_DIR`.
- History file: `<config-dir>/history.json` (max ~500 entries by default).
- Runtime globals and file captures are scoped per environment and document; they are released when you clear globals or switch environments.

---

## Examples

Explore `_examples/` for ready-to-run:

- `basic.http` - simple GET/POST with bearer auth.
- `scopes.http` - demonstrates global/file/request captures.
- `scripts.http` - pre-request and test scripting patterns.
- `graphql.http` - inline and file-based GraphQL requests.
- `grpc.http` - gRPC reflection and descriptor usage.
- `oauth2.http` - manual capture vs using the `@auth oauth2` directive.
- `transport.http` - timeout, proxy, and `@no-log` samples.
- `workflows.http` - end-to-end workflow with captures, overrides, and expectations.

Open one in Resterm, switch to the appropriate environment (`resterm.env.json`), and send requests to see each feature in action.

---

## Troubleshooting & Tips

- Use `Ctrl+P` to force a reparse if the request list seems out of sync with editor changes.
- If a template fails to expand (undefined variable), Resterm leaves the placeholder intact and surfaces an error banner.
- Combine `@capture request ...` with test scripts to assert on response headers without cluttering file/global scopes.
- Inline curl import works best with single commands; complex shell pipelines may need manual cleanup.
- `Ctrl+Shift+V` pins the focused response pane—ideal for diffing the last good response against the current attempt.
- Keep secrets in environment files or runtime globals marked as `-secret`. Remember that history stores the raw response unless you add `@no-log` or redact the payload yourself.

For additional questions or feature requests, open an issue on GitHub.
