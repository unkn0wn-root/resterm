# Captures

`@capture <scope> <name> <expression>` evaluates after the response arrives and stores the result for reuse.

Captures run in declaration order. A capture can read values produced earlier in the same batch through `vars.get`, `{{= ... }}`, or `{{name}}`. Values are stored only after every capture succeeds, so a failed batch leaves request, file, and global values unchanged.

Expressions can reference:

- `response.statusCode`, `response.statusText`, `response.text()`
- `response.headers["header-name"]` or `response.header("Header-Name")` for a single-valued header
- `response.json.path` shorthand (equivalent to `response.json().path`). `last.json.path` works the same way.
- `stream.kind()`, `stream.summary().sentCount`, `stream.summary().dropped`, and `stream.events()[0].text` for streaming transcripts (available when the request used `@sse` or `@websocket`). A non-zero `dropped` value means the retained transcript is incomplete.
- `vars.*`, `env.*`, `last.*`, imported `@use` modules, and other RestermScript helpers

Example:

```http
### Seed session
# @name AnalyticsSeedSession
# @capture global-secret analytics.sessionToken = response.json.sessionToken
# @capture file analytics.lastJobId = response.json.jobId
# @capture request analytics.trace = response.header("x-amzn-trace-id")
POST https://httpbin.org/anything/analytics/sessions
```

Template captures such as `{{response.json.token}}` still work and can be used next to RTS capture expressions. The template forms are:

- `{{response.body}}`, `{{response.status}}`, and `{{response.statusCode}}`
- `{{response.headers.X-Request-Id}}`. A header sent more than once gives its values joined with `, `.
- `{{response.json}}` for the whole body, or a path such as `{{response.json.items[0].id}}`. A negative index counts from the end, so `{{response.json.items[-1]}}` is the last item.
- `{{stream.kind}}`, `{{stream.summary.eventCount}}`, and `{{stream.events[-1].text}}` for streaming transcripts

`@capture` treats a value with a complete, unquoted `{{...}}` marker as interpolated text. Otherwise it parses the value as RestermScript. In text mode, surrounding characters such as `#` and unmatched brackets are literal. In script mode, `#` starts a comment.

An unquoted marker in a script comment switches the capture to text mode. Resterm warns when the remaining text looks like a function call, but it cannot reliably detect every case. Quote markers mentioned in comments:

```http
# @capture request x response.statusCode # mention "{{token}}"
```

Set `# @setting capture.strict true` to make capture-path misses fail instead of silently resolving to an empty string.

Do not mix unquoted template markers and RTS call syntax in the same capture expression (for example `contains({{name}}, "x")`). Use pure RTS (`contains(vars.get("name") ?? "", "x")`) or a template expression (`{{= contains(...) }}`).

`capture.strict` is the canonical key. `capture-strict` and `capture_strict` are accepted for compatibility. When multiple aliases are present, precedence is `capture.strict` > `capture-strict` > `capture_strict`.
