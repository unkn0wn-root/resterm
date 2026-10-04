# JavaScript hooks

Scripts use ES5.1 JavaScript. Each script block stops after 30 seconds or when the run is canceled. Scripts are not restricted to the workspace. See [Security](security.md).

## Script blocks (`@script`)

Add `# @script pre-request` or `# @script test` followed by lines that start with `>`.

```http
# @script pre-request
> var token = vars.global.get("reporting.token") || `script-${Date.now()}`;
> vars.global.set("reporting.token", token, {secret: true});
> request.setHeader("Authorization", `Bearer ${token}`);
> request.setBody(JSON.stringify({ scope: "reports" }, null, 2));
```

You can also use a brace block (`{% ... %}`) to avoid prefixing every line with `>`.

```http
# @script test
> {%
client.test("status ok", function () {
  tests.assert(response.statusCode === 200, "status code");
});
%}
```

Lines inside the block don't need `>`. A leading `>` is still stripped if present.
The `{% ... %}` block is only for inline script content. Script file includes must be written as their own `> < ./path.js` line outside the block.

Allowed example:

```http
# @script test
> < ./scripts/pre.js
> {%
client.test("ok", function () {});
%}
```

Disallowed example:

```http
# @script test
> {%
> < ./scripts/pre.js
client.test("ok", function () {});
%}
```

## Pre-request scripts (`@script pre-request`)

Objects:

- `request`
  - `getURL()`, `setURL(url)`
  - `getMethod()`, `setMethod(method)`
  - `getHeader(name)`, `setHeader(name, value)`, `addHeader(name, value)`, `removeHeader(name)`
  - `setBody(text)`
  - `setQueryParam(name, value)`
- `vars`
  - `get(name)`, `set(name, value)`, `has(name)`, `interpolate(text)`
  - `global.get(name)`, `global.set(name, value, options)`, `global.has(name)`, `global.delete(name)` (`options.secret` masks values)
- `console.log/warn/error` (no-op placeholders for compatibility)

The `set*` functions change the outgoing request and return no value. `removeHeader` also removes headers declared in the request file.

`getURL()` and `getHeader()` expand variable references and `{{= ... }}` expressions in values from the request file. They include variables set earlier in the script. An undefined reference throws an error.

A helper written directly in a field, such as `{{$uuid}}`, stays unchanged when a getter reads it. Writing it back also sends it unchanged. To use the same generated value in both the script and the request, declare it with `# @request id {{$uuid}}` and read it with `vars.get("id")`.

The `set*` functions and `addHeader` send the text you pass them. They do not expand variables, expressions, or helpers such as `{{$uuid}}`. A body line such as `@ path` is sent as text and does not read a file. Use `vars.get` to read a variable, or `vars.interpolate` to fill placeholders in a string:

```http
# @script pre-request
> request.setHeader("Authorization", "Bearer " + vars.get("token"));
> request.setHeader("X-Request-Id", vars.interpolate("{{$uuid}}"));
> request.setURL(request.getURL() + "?debug=1");
```

`vars.get` returns `{{= ... }}` expressions as text. Use a request getter to evaluate an expression in a request field, or calculate it in the script.

### Interpolating text

`vars.interpolate(text)` replaces `{{name}}` with the value from `vars.get(name)`. It sees variables set earlier in the script. Helpers such as `{{$uuid}}` generate a new value each time they appear. If a variable has the same name as a helper, its value is used.

```http
# @file base https://api.example.com
# @file token env:GITHUB_TOKEN

### Repos
# @name repos
# @script pre-request
> vars.set("userId", "42");
> request.setURL(vars.interpolate("{{base}}/users/{{userId}}/repos"));
> request.setHeader("Authorization", vars.interpolate("Bearer {{token}}"));
GET https://api.example.com
```

- It reads the same variables as `vars.get`. It cannot read `@const` values or OS variables without an `env:NAME` mapping.
- `{{...}}` inside an inserted value stays unchanged.
- Missing variables, `{{= ... }}` expressions, empty `{{ }}` placeholders, and placeholders without a closing `}}` throw an error. Calculate expressions in the script.
- Do not pass response text directly to `vars.interpolate`: it could contain `{{token}}` and read a secret. Store it with `vars.set` and insert it with `{{name}}` instead.

The same function is available in [RestermScript](rts/host-objects.md#vars).

Resterm warns when a value written by a pre-request script still contains `{{name}}`, `{{$helper}}`, or `{{= ... }}`. This applies to JavaScript and RTS. The warning includes the file and line of the setter call:

```text
api.http:12: Script sends {{token}} in header Authorization as written. Use vars.get("token").
api.http:13: Script sends {{$uuid}} in header X-Request-Id as written. Use vars.interpolate("{{$uuid}}").
```

The warning appears in the status bar, in Explain, and under the request in `resterm run`.

All `@script pre-request` blocks for a request share the same state. Each block sees changes made by earlier blocks through `vars.get`, `vars.global.get`, `getURL`, `getMethod`, and `getHeader`. RTS pre-request blocks run before JavaScript blocks, so their changes are visible too. Query parameters are different because they are merged into the URL after the scripts finish. So `getURL` does not show changes made by `setQueryParam`.

## Test scripts (`@script test`)

Objects:

- `client.test(name, fn)` - registers a named test. Exceptions or manual failures mark the test as failed.
- `tests.assert(condition, message)` - adds a pass/fail entry.
- `tests.fail(message)` - explicit failure.
- `response`
  - `status`, `statusCode`, `url`, `duration`
  - `body` (raw string)
  - `json()` (parsed JSON or `null`)
  - `headers.get(name)`, `headers.has(name)`, `headers.all` (lowercase map). For gRPC the map merges response metadata with the trailers, each trailer prefixed with `Grpc-Trailer-`, and `-bin` values arrive base64-encoded.
- `stream`
  - `enabled()` - returns `true` when the current response is an SSE or WebSocket transcript.
  - `kind()` - returns `"sse"` or `"websocket"`.
  - `summary()` - copy of the transcript summary. [WebSocket and SSE](streaming.md) lists its fields. Resterm fails the request when the stream fails, so a test does not need to check for that separately.
  - `events()` - array of event objects (`data`/`comment` for SSE, `type`/`text`/`base64`/`direction` for WebSockets).
  - `onEvent(fn)` - registers a callback that is called for each event after the script finishes. Useful for assertions over the whole stream.
  - `onClose(fn)` - registers a callback that is called once with the summary after all events replay.
- `vars` - same API as pre-request scripts. It reads request, file, and global values, and writes request-scope values for assertions.
- `vars.global` - same as in pre-request scripts. Changes persist after the script.
- `console.*` - same placeholders as above.

Example test block:

```http
# @script test
> client.test("captures token", function () {
>   var token = vars.get("oauth.manualToken");
>   tests.assert(!!token, "token should be available");
> });
```
