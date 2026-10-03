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
  - `get(name)`, `set(name, value)`, `has(name)`
  - `global.get(name)`, `global.set(name, value, options)`, `global.has(name)`, `global.delete(name)` (`options.secret` masks values)
- `console.log/warn/error` (no-op placeholders for compatibility)

The `set*` helpers do not return a value, but their changes still apply to the outgoing request. `removeHeader` can also remove headers declared in the request itself.

`getURL()` and `getHeader()` return values from the request file with variable references expanded, the same way `vars.get` returns them. Values set by the same script with `vars.set` are included, and `{{= ... }}` expressions are evaluated with them. A dynamic helper written directly in the field, such as `{{$uuid}}`, stays unexpanded because each use makes a new value. Declare it instead, as in `# @request id {{$uuid}}`, and the getter returns the value that is sent. If a reference is undefined, the call throws an error.

Values passed to the `set*` helpers are data. They are sent as written. Templates in them are not expanded, and `@ path` lines in a body are not read. Only dynamic helpers such as `{{$uuid}}` are rendered. Build values from variables instead of writing template text:

```http
# @script pre-request
> request.setHeader("Authorization", "Bearer " + vars.get("token"));
> request.setURL(request.getURL() + "?debug=1");
```

A `{{= ... }}` expression read through `vars.get` and written back is sent as text. Read it through a getter, or compute the value in the script.

When a value set by a pre-request script, in JavaScript or RTS, still holds a variable, an unknown `{{$...}}` helper, or a `{{= ... }}` template, Resterm shows a warning with the file and line of the call:

```text
api.http:12: Script sends {{token}} in header Authorization as written. Use vars.get("token").
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
