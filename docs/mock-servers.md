# Mock servers

Define mock responses beside your requests in `.http` / `.rest` files. Each scenario starts with a `###` separator, uses `@mock` to say which route it serves, and includes the HTTP response to return:

```http
### Payment accepted
# @mock method=POST path=/payments name=accepted default=true latency=150ms
HTTP/1.1 202 Accepted
Content-Type: application/json
X-Provider: sandbox

{"id":"pay_123","status":"pending"}
```

Serve a file or a workspace with `resterm mock payments.http`, or press `g Shift+M` in the TUI. [`resterm mock`](cli/mock.md) lists the command's flags, such as `--source` to serve only some files.

## Route and response syntax

- `method` and `path` are required. Write the path from the root, without a scheme or host (origin form). It may be exact (`/health`), match one segment (`/users/{id}`), or match the rest of the path (`/assets/{path...}`). Wildcard names must be unique within a path. A trailing slash changes which route matches.
- `name` is an optional scenario selector containing letters, digits, `.`, `_`, or `-`.
- `sequence` names a response sequence and follows the same naming rules. It cannot be combined with `name`.
- `sequence-key` gives each distinct `path`, `query`, `header`, or `cookie` value its own position in the sequence. It requires `sequence`.
- `default=true` marks the fallback for a route. A route can have at most one default, and a default cannot also have `@match` conditions.
- `latency` takes a non-negative duration such as `150ms` or `2s`, or a [distribution](#response-latency) that changes the delay from request to request. Waiting stops when the client cancels the request.
- Response interpolation is enabled by default. Set `interpolate=false` to preserve `{{...}}` as literal response text.
- The response status must be `200` through `599`. Repeated headers are preserved. Connection/framing headers such as `Content-Length`, `Transfer-Encoding`, and `Connection` are managed by the server and rejected in source files.
- The body is literal text through the next `###`, or a single `< ./fixtures/body.json` file reference. Relative fixture paths resolve from the request file that declares them and are watched for hot reload. They must stay inside the selected request file's directory or workspace root. Absolute paths and paths or symlinks that escape that root are rejected.
- `HEAD` returns the selected status and headers without a body. `204`, `205`, and `304` scenarios must not declare a body.

## Response latency

`latency=150ms` delays every response by the same amount. To vary the delay between requests, use a distribution. This lets you test how a client handles timeouts, retries, and backoff:

```http
# @mock method=GET path=/slow latency=random(100ms,500ms)
# @mock method=GET path=/steady latency=normal(250ms,50ms)
# @mock method=GET path=/jittery latency=jitter(200ms,20%)
```

| Form | Delay |
| --- | --- |
| `150ms` | The same every time. |
| `random(min,max)` | Anywhere between the two bounds, evenly spread. `max` cannot be shorter than `min`. |
| `normal(mean,stddev)` | Around `mean`, most of the time within `stddev` of it, occasionally further out. |
| `jitter(base,spread)` | Anywhere between `base - spread` and `base + spread`. |

Durations are written as they are everywhere else in Resterm: `250ms`, `1.5s`, `2m30s`. The second argument of `normal` and `jitter` can be a percentage of the first instead of a duration, so `jitter(200ms,20%)` stays between `160ms` and `240ms`, and `normal(250ms,20%)` spreads by `50ms`. When the result is below zero, the response is not delayed at all.

Spaces between arguments are fine, as in `random(100ms, 500ms)`, and the name is case-insensitive. A value that is neither a duration nor a known distribution is reported with the mock's other parse errors.

## Response interpolation

Response header values and inline or file-backed bodies can use request data and Resterm's dynamic template helpers:

```http
### User
# @mock method=POST path=/users/{id}
HTTP/1.1 200 OK
Content-Type: application/json
X-Request-ID: {{$uuid}}

{
  "id": {{json.path.id}},
  "view": {{json.query.view}},
  "tenant": {{json.headers.X-Tenant}},
  "name": {{json.body.user.name}},
  "attempt": {{json.body.attempt}},
  "generatedAt": "{{$timestampISO8601}}"
}
```

Available inputs are:

- `{{path.name}}` for a route wildcard, including catch-all wildcards.
- `{{query.name}}` for a query value. Repeated values use the first value.
- `{{headers.Name}}` for a request header, including `{{headers.Host}}`. Header names are case-insensitive and repeated values use the first value.
- `{{body.path.to.field}}` for a JSON body field. Array indexes and quoted keys use the same forms as `items[0].id` and `$["display.name"]`.
- Every [dynamic helper](variables.md#dynamic-helpers), such as `{{$uuid}}`, `{{$timestampISO8601 - 1h}}`, `{{$randomChoice("queued", "done")}}`, and `{{$fake.company}}`. A helper reference that names no helper, or one called the wrong way, fails when the mock set compiles.

The placeholders above insert text as-is. Use them in response headers and in plain text or XML bodies. Strings read from a JSON request body are not quoted or escaped. Other JSON values such as numbers, booleans, `null`, arrays, and objects are written as compact JSON. Each occurrence is resolved separately.

When a request value is part of a JSON response, add the `json.` prefix:

- `{{json.path.name}}`, `{{json.query.name}}`, and `{{json.headers.Name}}` turn their values into JSON strings. Resterm adds the surrounding quotes and escapes the contents.
- `{{json.body.path.to.field}}` keeps the field's JSON type and numeric precision.

Do not put quotes around a `json.` placeholder. It writes a complete JSON value. For example:

```json
{"id": {{json.path.id}}}
```

If `id` is `alice`, Resterm returns:

```json
{"id":"alice"}
```

Quotes, backslashes, and control characters in the value are escaped so they cannot change the surrounding JSON. Resterm only encodes the placeholder value. It does not infer this behavior from `Content-Type` or validate the finished response body. A mock can still return malformed JSON when a test needs it.

Template names and paths are validated when the mock set compiles. A missing query value, header, or JSON field returns `400 application/problem+json`. Malformed JSON also returns `400`, and JSON bodies over 4 MiB return `413`. The server renders every response header and the complete body before writing anything. A rendered header value that is not HTTP-safe or a failed dynamic helper returns `500`. `HEAD` still renders the selected body to calculate its `Content-Length`, but does not write the body.

Use `interpolate=false` when a mock must return template syntax literally. Captured responses and OpenAPI-generated mocks add this option automatically when their static response contains `{{`.

## Response sequences

A named sequence contains two or more raw responses separated by a line whose trimmed content is exactly `---`:

```http
### Poll payment
# @mock method=GET path=/payments/{id} sequence=polling sequence-key=path.id
HTTP/1.1 503 Service Unavailable
Retry-After: 1

{"status":"pending"}
---
HTTP/1.1 503 Service Unavailable
Retry-After: 1

{"status":"pending"}
---
HTTP/1.1 200 OK

{"status":"completed"}
```

Each ordinary request reserves the next response in the sequence, even when requests arrive at the same time. Once the last response is reached, it repeats. Without `sequence-key`, all clients and wildcard path values share one position (cursor) per compiled scenario. `@match`, `default`, and `latency` apply to the whole sequence.

Resterm advances the cursor before waiting for latency, filling placeholders, or writing the response. If the client cancels or rendering or writing fails, the reserved response is still consumed.

Use one of these key sources when callers should advance independently:

```http
# @mock method=GET path=/payments/{id} sequence=polling sequence-key=path.id
# @mock method=GET path=/jobs sequence=polling sequence-key=query.job
# @mock method=GET path=/jobs sequence=polling sequence-key=header.X-Correlation-ID
# @mock method=GET path=/jobs sequence=polling sequence-key=cookie.session
```

Path keys must name a wildcard declared by that route. Query, header, and cookie keys use the first repeated value. A missing, empty, or larger-than-4-KiB key returns `400` and does not advance a cursor. `X-Resterm-Mock-Status` still pins a response without consulting the key or advancing it. Each compiled sequence keeps at most 10,000 distinct keys by default. A new key past that limit returns `429`, and existing keys are not evicted. Change the limit with `resterm mock --sequence-key-limit`.

`X-Resterm-Mock: polling` selects the sequence. `X-Resterm-Mock-Status: 200` pins the first sequence response with that status without advancing the cursor, and it can be combined with the name selector. Sequence progress appears in CLI and TUI request logs as `polling 2/3`.

Reset every cursor, or every cursor with a particular sequence name, without restarting the server. In the TUI, use `:mock reset` or `:mock reset polling`. For a standalone server, run `resterm mock reset` or `resterm mock reset polling` ([mock operations](cli/mock.md#mock-operations)). A named reset applies to every route using that sequence name. Requests that already reserved a response finish with that response. Later requests start from the reset state.

A no-op hot reload keeps the active handler and its cursors. Any source or fixture change that produces a new compiled handler resets every sequence to its first response. In a sequence's inline body, a line whose trimmed content is exactly `---` is reserved as the response delimiter. Use a file-backed body when the payload must contain such a line. Outside a sequence, `---` remains ordinary body text. A `---` with no response after it at the end of the block is reported as a dangling delimiter.

## Conditional scenarios

Add `@match` before the raw status line to select a response by query values, request headers, or the JSON request body:

```http
### Declined payment
# @mock method=POST path=/payments name=declined
# @match query={"mode":"decline"} headers={"X-Tenant":"demo"} json={"amount":0}
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json

{"error":"amount must be positive"}
```

Query and header matchers share the same shorthand and the same rule objects:

```http
# @match headers={"X-Tenant":{"exact":"demo"},"Authorization":{"prefix":"Bearer "},"X-Correlation-ID":{"present":true},"X-Debug":{"absent":true}}
# @match headers={"User-Agent":{"contains":"Chrome"},"X-Version":{"regex":"^v[0-9]+$"},"X-Env":{"oneOf":["dev","stage","prod"]}}
# @match query={"channel":{"oneOf":["web","ios"]},"page":{"gte":2},"trace":{"absent":true}}
```

- A string or array is shorthand for `exact`. Repeated values must match exactly and in order.
- `prefix` succeeds when any value starts with the non-empty prefix.
- `contains` succeeds when any value contains the non-empty substring.
- `regex` succeeds when any value matches the [RE2](https://pkg.go.dev/regexp/syntax) pattern. Matching is unanchored, so use `^` and `$` to match a whole value, and `(?i)` to ignore case.
- `oneOf` succeeds when any value equals one of the listed values. The list cannot be empty. Unlike `exact`, order and extra values do not matter.
- `present` requires a value, including an explicitly empty one.
- `absent` requires no value at all.
- `gt`, `gte`, `lt`, and `lte` are query-only. Each succeeds when any value reads as a number and compares that way against the operand.

Each key takes exactly one rule. Header names ignore case; query parameter names and all values are case-sensitive.

Except for `exact`, `present`, and `absent`, each rule checks repeated values individually and passes if any one matches. A comma-separated list inside one value is not split, and a missing key fails these rules. Empty `prefix`, `contains`, and `regex` operands and empty `oneOf` arrays are rejected. To match an empty value, write `{"regex":"^$"}`.

The numeric operand must be written as a JSON number, so `{"gte":2}` rather than `{"gte":"2"}`. A value that does not read as a number is an ordinary non-match, not an error, so `?page=none` fails `{"gte":2}`.

Only declared query and header keys are constrained. Numbers are compared by decimal value, so `1`, `1.0`, and `1e0` are equal. Exponents that are too large are capped instead of overflowing. Two values beyond the same limit compare as equal.

### Matching the request body

Use `json` to match a literal body subset and `json-rules` to compare body values. They can be combined:

```http
# @match json={"type":"order"}
# @match json-rules={"amount":{"gt":100},"user":{"age":{"gte":18}},"status":{"oneOf":["new","hold"]}}
```

All body, query, and header conditions must match. Body matching accepts `application/json` and `+json` media types. A malformed body returns `400`, and a body larger than 4 MiB returns `413`.

Everything inside `json` is request data:

- Objects match as recursive subsets, so members the pattern does not mention are ignored.
- Arrays match exactly and in order.
- Other values match by value.
- Keys are always field names. Patterns such as `{"$gt":100}`, `{"$schema":"..."}`, `{"$ref":"#/$defs/user"}`, and `{"gt":125}` need no escaping.

To match a body containing a JSON string, quote the JSON value inside the option value:

```http
# @match json='"paid"'
```

The single quotes delimit the option value. The double quotes are part of the JSON string. For example, `json=100` matches the number `100`, while `json='"100"'` matches the string `"100"`. Other JSON values need no extra quotes. Resterm writes this matcher in the same form.

The structure of `json-rules` follows the request body:

- `gt`, `gte`, `lt`, and `lte` require JSON numbers on both sides. The string `"101"` does not match `{"gt":100}`.
- `oneOf` matches any listed JSON value. Object and array entries must match the entire value.
- Several operators can apply to one value. For example, `{"amount":{"gte":100,"lt":500}}` checks both bounds.
- Missing fields and values of the wrong type do not match.
- Field names containing `.`, `/`, `$`, or non-ASCII text need no escaping. `{"a.b":{"gt":1}}` refers to the top-level field named `a.b`.

Operator names can also be body field names. Nest another rule inside the field to match one:

```http
# Matches {"range": {"gt": 125}}
# @match json-rules={"range":{"gt":{"gt":100}}}
```

The outer `gt` is a field because its value is an object. The inner `gt` is the operator.

Empty rule objects, unknown operators, invalid operands, and empty `oneOf` arrays are configuration errors. The error includes the location:

```text
invalid json-rules matcher at amount.gtt: expected a rule object or a known operator
```

Rules do not inspect array elements. Match an array with `json`, or compare the whole array with `oneOf`:

```http
# @match json-rules={"roles":{"oneOf":[["admin"],["admin","auditor"]]}}
```

### Splitting a long matcher

If a quoted or bracketed value remains open, the matcher continues on the next comment line:

```http
# @match json-rules={
#   "amount": {"gte": 100, "lt": 500},
#   "user": {
#     "age":  {"gte": 18},
#     "tier": {"oneOf": ["gold", "silver"]}
#   },
#   "status": {"oneOf": ["new", "hold"]}
# }
```

You can also split unrelated fields across declarations:

```http
# @match json-rules={"amount":{"gte":100,"lt":500}}
# @match json-rules={"status":{"oneOf":["new","hold"]}}
```

Resterm merges repeated object values. Repeating a field or a non-object value is an error. When Resterm rewrites the file, it writes the matcher as one merged line.

Resterm selects a scenario in this order:

1. `X-Resterm-Mock: <name>` selects a named scenario directly.
2. `X-Resterm-Mock-Status: <code>` limits candidates to a response status. It can be combined with the name selector. For a sequence, it pins the first matching step without advancing.
3. Otherwise, conditional scenarios are checked in file/path order, then the explicit default, then the first unconditional scenario.
4. A missing route, selector, or match returns an `application/problem+json` `404` response.

## Request verification

Add one exact call-count expectation before the first mock response. It inherits that block's method, path, and optional `@match` conditions:

```http
### Payment webhook
# @mock method=POST path=/webhooks/payment
# @match headers={"Authorization":{"prefix":"Bearer "}} json={"status":"completed"}
# @expect calls=1
HTTP/1.1 204 No Content
```

`calls=0` asserts that an endpoint was not called. An expectation counts the journaled requests that match its whole pattern. It does not depend on which scenario or response was selected. Two expectations that overlap can both count the same request.

For a standalone server, run `resterm mock verify payments.http` from another terminal. [Mock operations](cli/mock.md#mock-operations) covers its flags and exit codes.

While the TUI runs a mock server, `:mock verify` checks the active compiled expectations and opens a result view. Request assertions and workflows can read the same journal through RestermScript:

```http
# @assert mock.count({method:"POST", path:"/webhooks/payment"}) == 1
# @assert mock.received({method:"POST", path:"/webhooks/payment", headers:{Authorization:{prefix:"Bearer "}}, json:{status:"completed"}})
```

The pattern fields work like the matching `@match` options. [The `mock` object](rts/host-objects.md#mock) describes them. These helpers inspect the TUI's active mock server only, so use `resterm mock verify` for standalone or headless automation.

## CORS, reload, and request journals

CORS follows the `--cors` flag of [`resterm mock`](cli/mock.md). A declared `OPTIONS` mock takes precedence over automatic preflight handling.

Resterm watches source and fixture files by default. After a change, it builds a complete new set of routes and replaces the old set in one operation. If parsing or compilation fails, the last valid routes keep serving. Reloading keeps the request journal, while verification uses the new expectations.

The access log shows recent traffic; the verification journal stores requests for call-count checks. They are separate and both have limits. By default, the log keeps 200 entries and the journal keeps 2000. The journal also limits stored data to 16 MiB and keeps up to 64 KiB of each request body. Change its limits with `--journal-entries`, `--journal-bytes`, and `--journal-body-limit`.

The journal records matched, unmatched, and method-not-allowed requests. It excludes CORS preflights and private operational calls. If an entry is dropped or cannot be stored, verification fails rather than reporting a potentially wrong count. A pattern that checks only metadata can still inspect a request whose stored body was truncated. A JSON pattern reports the journal as incomplete for that request.

Inside the TUI:

- `g Shift+M` toggles the mock server at the remembered session address (initially `127.0.0.1:8080`).
- `:mock`, `:mock status`, `:mock start [host:port]`, `:mock stop`, and `:mock restart [host:port]` manage it. Both `start` and `restart` also accept the address as `--addr host:port`, and take the same flag names, short aliases, and `--flag=value` form as the `resterm mock` CLI.
- `:mock start` serves every request file in the workspace. `--source` narrows that to named files, either by repeating the flag or by passing a comma-separated list. After `--source`, `-s`, or their `--flag=value` forms, the command popup lists workspace directories and `.http` / `.rest` files. In a comma list, only the last path is completed. Paths containing whitespace are quoted automatically. Because commas separate sources, a source filename itself cannot contain a comma. Paths resolve against the workspace root and must stay inside it. `--recursive` includes subdirectories and `--all` names the full-workspace default. Neither combines with `--source`.
- With `--source`, only the listed files reload. File-based response bodies still resolve relative to each file and stay confined to the workspace root.
- The scope is remembered like the address, so `:mock restart`, a later `:mock start`, and `g Shift+M` keep serving the same files with the same recursion. Naming a scope again is what changes it: `--source` narrows, `--all` returns to the whole workspace, and `--recursive` adds subdirectories. `:mock status` names the remembered files while the server is stopped, and changing the workspace forgets the scope.
- `:mock logs` opens the request log, where `c` clears the log. `:mock clear` clears both the log and the verification journal.
- `:mock reset [sequence]` resets sequence cursors, and `:mock verify` checks active `@expect` declarations.
- While the server runs, the header shows its compact source scope when space allows. The status bar shows the active address, route count, call count, and reload-error marker.
- The active editor buffer overlays its on-disk file during reload, so unsaved mock edits can be tested. Invalid edits keep the last valid routes.

Press `g a` or run `:mock capture` to append the focused live or pinned HTTP response as a mock block. Capture keeps the status, ordinary headers, raw text body, method, and URL path. It does not add query, header, or body matchers, or latency. The editor jumps to the new, unsaved block so you can review it.

Inline capture does not accept persisted history entries, binary or non-UTF-8 bodies, bodies over 4 MiB, lines longer than the parser allows, or bodies containing a `###` separator. Check captured headers and bodies for credentials or personal data before saving.

OpenAPI imports can create the same blocks with `--openapi-mode mocks` or combine requests and mocks with `--openapi-mode both`.
