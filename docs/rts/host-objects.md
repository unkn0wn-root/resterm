# Host objects

Resterm exposes host objects when evaluating templates, directives, `@apply`, assertions, and pre-request scripts. In pre-request scripts, `request` and `vars` expose mutation helpers. Other available objects are read-only. Lookups in `env` and `vars` are case-insensitive and ignore surrounding whitespace. Header lookups are case-insensitive, while query parameters and JSON paths are exact. See [Keys and names](language.md#keys-and-names). The TUI also exposes `mock` during request evaluation. Its helpers work while the workspace mock server is running and return an error when it is stopped.

## Name precedence

Every evaluation binds its names in a fixed order, and a later layer wins:

1. The standard library and the host objects below.
2. Host extensions such as `mock`. These are additive and cannot reuse a name from the first layer.
3. `@use` aliases. These cannot reuse a name from either layer above.
4. Local values, currently the `@for-each` loop variable. These shadow every layer above, so `# @for-each [1,2] as json` makes `json` the loop item for that request and hides the `json` namespace.
5. The `@assert` shorthands, inside assertions only. `status`, `statusCode`, `statusText`, `header(name)`, and `text()` always refer to the response under test, so a loop variable named `status` is still the loop item everywhere except inside an `@assert`.

Reserved words are never bindable at any layer. The parser rejects them in `@for-each` and `@use`, and the runtime rejects them again.

Shadowing applies to expressions. A module or an `@rts pre-request` block still cannot declare a name that is already bound, so `let json = 1` has always been an error. A local adds its own name to that rule. A request with `# @for-each [1,2] as item` cannot also declare `let item` in its pre-request block. Rename the declaration or the loop variable.

## env

`env` provides environment values. You can access values through `env.get("key")`, `env.has("key")`, `env.require("key"[, msg])`, or `env.key`. `require` throws when a value is missing.

`env.meta.name` is the selected environment, and `env.meta.groups` holds the selected profile for each group. Metadata is separate from the environment value namespace, so an environment value may be called `name`, `groups`, or `meta` without ambiguity.

For grouped environments, `env.meta.name` is the complete selection label, such as `api=dev, app=dev app 1`. Group names use the same matching rules as environment names, and the same `get`, `has`, and `require` helpers are available:

```rts
env.meta.groups.api
env.meta.groups.get("credentials")
env.meta.groups.has("app")
env.meta.groups.require("api", "API profile is required")
```

For named environments, `env.meta.name` is the environment name and `env.meta.groups` is empty.

```rts
env.meta.name     # dev, the selected environment
env.get("name")   # the environment variable called name, if declared
env["name"]       # the same variable
env.NAME          # the same variable, since env names are case-insensitive
```

A key mapped from an OS variable with `env:NAME` appears under its declared name, and the OS variable's own name does not. See [Values from OS environment variables](../variables.md#values-from-os-environment-variables).

`env` only contains the selected environment. A file declaration such as `# @file token env:RESTERM_TOKEN` is available through `vars.get("token")` and `{{token}}`, but not `env.get("token")`.

## vars

`vars` provides request runtime variables, including globals and workflow overrides. You can access values through `vars.get("key")`, `vars.has("key")`, `vars.require("key"[, msg])`, or `vars.key`. `vars.global` provides global reads and writes in pre-request scripts through `get`, `has`, `require`, `set`, and `delete`.

`vars` contains values a run can override, so `@const` values and unmapped OS variables stay template-only. Everything else, including `env:NAME` mappings, follows the [variable resolution order](../variables.md#variable-resolution-order).

`vars.interpolate(text)` fills placeholders in a string using the same [rules as JavaScript](../scripting.md#interpolating-text). It also works in `@apply`:

```http
# @apply {url: vars.interpolate("{{base}}/users/{{id}}")}
```

To read a variable named `interpolate`, use `vars.get("interpolate")`.

Values written by scripts, captures, or workflow steps are plain data. Text beginning with `env:` in a runtime value stays literal.

## request

`request` provides a summary of the current request. It exposes `method`, `url`, `headers`, `header(name)`, and `query`. `headers` and `query` are `dict<string, string | list<string>>`: one value is a string, multiple values are a list, and zero values are an empty list. Header keys are lowercased, while query keys are exact. `header(name)` is a shortcut that returns the first value. A document header whose name is not an HTTP field name is rejected before evaluation. In `@rts pre-request` blocks, mutation helpers are available, including `request.setMethod`, `request.setURL`, `request.setHeader`, `request.addHeader`, `request.removeHeader`, `request.setQueryParam`, and `request.setBody`. Their string arguments are strict, so convert numbers or booleans with `str(...)` yourself. The longer `@script pre-request lang=rts` form works the same way. In `@apply`, the request object is read-only, so you return a patch dict instead of changing it.

In `@apply` and pre-request blocks, `url`, `headers`, `header(name)`, and `query` expand variables and `{{= ... }}` expressions from the request file, just as `request.getURL()` does in JavaScript. Values set by scripts are shown unchanged. An undefined reference is an error.

Values passed to `request.set*` or `request.addHeader`, or returned in a patch, are sent unchanged. This includes `{{$...}}` helpers. Use `vars.interpolate` to fill placeholders before writing a value.

## last

`last` provides a summary of the most recent response. It exposes `status`, `statusCode`, `statusText`, `url`, `headers`, `header(name)`, `text()`, and `json(path)`. Header values use the same cardinality-based representation as `request.headers`, and `header(name)` returns the first value. `json(path)` accepts a simple dot and `[index]` path (optional leading `$`) and returns null when a value is missing.

For gRPC responses, `headers` merges the response metadata with the trailers, each trailer prefixed with `Grpc-Trailer-`. Values under keys ending in `-bin` are binary and read back base64-encoded without padding, the way they travel on the wire, so a trailer sent as `x-trace-bin` reads as `last.header("grpc-trailer-x-trace-bin")`.

## response

`response` has the same shape as `last`. It is available after the current request completes in `@assert` and both forms of `@capture`, including a `{{= ... }}` expression inside one.

Other expressions run before the current request completes. Use `last` for the previous response in URLs, headers, bodies, and conditions such as `@when`, `@skip-if`, `@for-each`, `@if`, and `@switch`. Using `response` in those contexts is an error that `try` cannot catch.

## trace

`trace` provides timing and budget information for the most recent response. It includes helpers such as `trace.enabled()`, `trace.durationMs()`, `trace.durationSeconds()`, `trace.durationString()`, `trace.error()`, `trace.started()`, `trace.completed()`, `trace.phases()`, `trace.phaseNames()`, `trace.getPhase("dns")`, `trace.budgets()`, `trace.breaches()`, `trace.withinBudget()`, and `trace.hasBudgets()`.

## stream

`stream` provides information about SSE and WebSocket requests. Its helpers include `stream.enabled()`, `stream.kind()`, `stream.summary()`, and `stream.events()`. The summary and event fields depend on the stream type. SSE summaries include `eventCount`, `byteCount`, `duration`, `reason`, `error`, `errorClass`, and `dropped`. WebSocket summaries include `sentCount`, `receivedCount`, `duration`, `closedBy`, `closeCode`, `closeReason`, `errorClass`, and `dropped`. When a stream fails, `errorClass` names the kind of failure and decides the detailed exit code. A non-zero `dropped` value means that some events are missing from the transcript. If a stream fails, its request fails too.

## mock

When the TUI's workspace mock server is running, `mock` provides read-only access to its bounded request journal:

```rts
mock.count({method: "POST", path: "/webhooks/{name}"})
mock.received({
  method: "POST",
  path: "/webhooks/payment",
  query: {page: {gte: 2}, channel: {oneOf: ["web", "ios"]}},
  headers: {
    Authorization: {prefix: "Bearer "},
    "X-Version": {regex: "^v[0-9]+$"},
    "X-Env": {oneOf: ["dev", "prod"]}
  },
  json: {status: "completed"},
  jsonRules: {amount: {gt: 100}, user: {age: {gte: 18}}}
})
```

Both helpers require one pattern dictionary. `count` returns the exact number of matching requests, and `received` returns true when that count is greater than zero. Pattern fields are optional:

- `method` is case-insensitive and normalized to uppercase.
- `path` uses mock path syntax, including `{name}` and terminal `{name...}` wildcards.
- `query` maps case-sensitive names to exact strings or lists, or to one rule. It accepts every header rule below plus `{gt: 10}`, `{gte: 10}`, `{lt: 10}`, and `{lte: 10}`, which read each value as a number and treat anything else as a non-match.
- `headers` maps case-insensitive names to exact strings or lists, or to one rule: `{exact: ...}`, `{prefix: "..."}`, `{contains: "..."}`, `{regex: "..."}`, `{oneOf: [...]}`, `{present: true}`, or `{absent: true}`. Header values are case-sensitive, `regex` uses unanchored RE2 syntax, and `oneOf` needs a non-empty array.
- `json` matches a literal body subset. Objects use recursive subset matching, while arrays are exact and ordered. Every key is a field name, including `$schema`, `$ref`, and `gt`.
- `jsonRules` follows the body structure and supports `{gt: ...}`, `{gte: ...}`, `{lt: ...}`, `{lte: ...}`, and `{oneOf: [...]}`. Numeric comparisons require numbers on both sides. Several operators can apply to one value, and `oneOf` compares entire objects and arrays. When `json` and `jsonRules` are both present, both must match.

If entries were evicted from the journal, these helpers fail instead of returning a result that might be wrong. Resterm does not connect `resterm run` to an external journal. For standalone runs or CI, use `@expect` entries with `resterm mock verify`.
