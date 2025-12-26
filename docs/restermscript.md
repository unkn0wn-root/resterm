# RestermScript Technical Reference

RestermScript (or RTS which you will see quite often throughout the docs) is Resterm's built in expression language for templates, directives, and reusable modules. It is designed to be small, bounded, and easy to review inside request files. JavaScript via Goja is still available, but RestermScript is the preferred option when you want predictable behavior, clear errors, and safe execution.

## Why this even exists

- RTS is bounded and predictable because expressions run with strict step limits, cannot perform network operations or file writes, and only read files via `json.file` when file access is enabled.
- RTS is safe because it avoids arbitrary evaluation and does not expose system APIs.
- RTS is clear because the syntax is small and purpose built for request files.
- RTS is debuggable because errors include file, line, and column information along with a call stack.

## When to use it

Use RestermScript when you need small, safe logic for request evaluation and control flow.

- Template values such as `{{= expr }}` are a good fit when you want computed headers, URLs, or JSON bodies.
- Request and workflow control directives such as `@when`, `@skip-if`, `@if`, `@switch`, and `@for-each` can be driven by RestermScript expressions.
- Assertions using `@assert` are readable and produce clear failures.
- Reusable `.rts` modules imported with `@use` let you share logic across requests without bringing in JavaScript.

Use JavaScript only when you need full language features or when porting existing logic is not worth the rewrite.

## Where it runs

1) Templates

```
Authorization: Bearer {{= vars.get("auth.token") ?? env.get("auth.token") }}
```

Templates evaluate expressions and insert their string results into request fields. They are read only and should not cause side effects.

2) Directives

```
# @when env.has("feature")
# @assert response.statusCode == 200
```

Directives evaluate expressions to decide whether a request runs or whether an assertion passes. They are read only and should not mutate request state.

3) Modules

```
# @use ./rts/helpers.rts as helpers
```

Modules are compiled once and expose only exported names through the alias. Modules execute with stdlib; when the host provides a `request` object it is available (read-only outside pre-request scripts). Modules do not automatically see `env`, `vars`, `last`, `response`, `trace`, or `stream`, so pass values into module functions explicitly.

4) Apply patches

```
# @apply {headers: {"X-Test": "1"}}
```

Apply patches evaluate a single RestermScript expression that returns a patch dict and applies it to the outgoing request. They run before pre-request scripts and use read-only `request` and `vars` objects.

5) Pre request scripts

```
# @script pre-request lang=rts
```

Pre-request scripts run full RestermScript blocks and can mutate the outgoing request and variables. They run before JavaScript pre-request blocks.

## Language overview

### Comments

`#` starts a comment that runs to the end of the line. It can appear after whitespace or code.

### Blocks and statement endings

Blocks use `{ ... }` and group statements together. A newline can end a statement when the previous token can finish a statement. Newlines inside `()` and `[]` are ignored. The language also accepts the semicolon token as a statement terminator, but this guide uses newlines for clarity.

### Identifiers and keywords

Identifiers start with a letter or `_` and can contain letters, digits, and `_` characters. The language reserves keywords and they cannot be used as identifiers.

Keywords:

```
export fn let const if elif else try return for break continue range
true false null and or not
```

### Literals

```
null
true / false
123  3.14
"string"  'string'
[1, 2, 3]
{a: 1, "b": 2}
```

String escapes include `\n`, `\r`, `\t`, `\\`, `\"`, and `\'`. Dict keys in literals are identifiers or quoted strings, and dict keys are always strings at runtime.

### Operators by precedence

- Postfix operators include function calls, indexing, and member access.
- Unary operators include `not`, `try`, and unary `-`.
- Multiplicative operators include `*`, `/`, and `%`.
- Additive operators include `+` and `-`.
- Comparison operators include `<`, `<=`, `>`, and `>=`.
- Equality operators include `==` and `!=`.
- Logical operators include `and` and `or`.
- The coalesce operator `??` returns the right side when the left side is null.
- The ternary operator `cond ? a : b` selects between two values.

`+` adds numbers or concatenates strings. Non numeric values are converted to string using `str()`. Comparisons only work for numbers or strings, and equality only works for primitive types.

### Error handling with try

```
try expr
```

The `try` operator evaluates its expression and returns an object with `ok`, `value`, and `error` fields. `ok` is true on success and false on error. `value` holds the result on success and is null on error. `error` is a single line error string on failure and null on success. It does not catch hard aborts such as step limits, timeouts, or cancellations. You can use `try expr` directly in conditionals, but checking `r.ok` is often clearer.

Example:

```
let r = try json.file("_data/users.json")
if not r.ok { return [] }
return r.value
```

Use in `.http` expressions and directives:

```
# @when try json.file("_data/flags.json")
# @for-each ((try json.file("_data/users.json")).value ?? []) as user
# @assert try response.json("data")

Authorization: Bearer {{= (try response.json("auth.token")).value ?? "" }}
```

This pattern is most useful for optional files, optional JSON bodies, or helper calls that may fail.

### Types and truthiness

RTS has several runtime types.

- Null represents the absence of a value.
- Bool represents true or false.
- Number uses float64 for numeric values.
- String stores UTF 8 text.
- List stores ordered values.
- Dict stores key value pairs.
- Function represents a callable value.
- Object represents host objects provided by Resterm.

Truthiness follows consistent rules. Null, false, zero, the empty string, the empty list, and the empty dict are false. All other values are true unless a host object defines custom truthiness (for example, `try` results are truthy only when `ok` is true).

### Indexing and member access

List indexing uses numeric indices such as `list[0]`, and out of range accesses return null. Dict access uses `dict["key"]` or `dict.key`, and missing keys return null. Object member access is supported, while indexing depends on the object implementation.

## Statements

### let and const

```
let name = expr
const name = expr
```

`let` creates a mutable binding and `const` creates an immutable binding. Redeclaring a name in the same scope is an error, while shadowing a name in an inner block is allowed. Assignment requires the name to exist in the current or parent scope.

### Assignment

```
name = expr
```

Assignment only applies to variable names. Member assignment and index assignment are not supported.

### Functions

```
fn add(a, b) {
  return a + b
}
```

Functions close over their lexical environment. Function names are immutable because `fn` defines a constant binding. Function parameters are local variables and can be reassigned.

### Conditionals

```
if cond {
  ...
} elif other {
  ...
} else {
  ...
}
```

Conditionals evaluate each branch in order and execute the first branch whose condition is true. The `else` branch runs only when no earlier condition is true.

### for loops

RTS supports several loop forms.

```
for { ... }
for cond { ... }
for let k, v range expr { ... }
```

The language also supports a three clause loop with init, condition, and post clauses. The clauses are separated by the semicolon token.

Rules for loops are consistent. `break` and `continue` are valid only inside loops. `const` is not allowed in loop headers. `for let` introduces loop scoped variables that do not escape the loop block. `for range` without `let` assigns to existing variables.

### range semantics

Range iteration is deterministic and follows clear rules.

- When you range a list, the key is the index and the value is the item.
- When you range a dict, the key is the string key and the value is the item, and keys are sorted to keep output stable.
- When you range a string, the key is the byte index and the value is a single rune string.

Example:

```
for let i, ch range "go" {
  // i is the byte index, ch is "g" and then "o"
}
```

## Modules and exports

Modules are `.rts` files and they are imported with `@use`.

- `export` exposes a name from a module.
- `@use ./path.rts as alias` imports a module into a request or file.
- Aliases are required because they avoid name collisions and make references explicit.
- Modules are cached, so top level mutable state can persist across runs.

Example:

```rts
// helpers.rts
export fn authHeader(token) {
  return token ? "Bearer " + token : ""
}
```

```http
# @use ./rts/helpers.rts as helpers
Authorization: {{= helpers.authHeader(vars.get("auth.token")) }}
```

Modules run with stdlib only. The `request` object is available when the host provides it, but `env`, `vars`, `last`, `response`, `trace`, and `stream` are not. Pass values in as arguments when you need extra context.

## Stdlib

RTS provides a small standard library that covers common request needs without enabling file writes or network access. It keeps expressions small, readable, and predictable. The stdlib is available as `stdlib`; core helpers and namespaces (`base64`, `url`, `time`, `json`, `headers`, `query`) are also exposed at top level for convenience. `text`, `list`, `dict`, and `math` are available only under `stdlib`.

### Core helpers

- `stdlib.fail(msg)` stops evaluation and returns an error message.
- `stdlib.len(x)` returns the length of a string, list, or dict.
- `stdlib.contains(haystack, needle)` checks whether a value is contained in a string, list, or dict.
- `stdlib.match(pattern, text)` applies a regular expression to text and returns true when it matches.
- `stdlib.str(x)` converts a value to a string, using JSON for lists and dicts.
- `stdlib.default(a, b)` returns `a` unless it is null, otherwise it returns `b`.
- `stdlib.uuid()` generates a UUID and requires random generation to be enabled.

### Encoding and URL helpers

- `stdlib.base64.encode(x)` encodes a string to base64.
- `stdlib.base64.decode(x)` decodes a base64 string.
- `stdlib.url.encode(x)` percent encodes a string for URL use.
- `stdlib.url.decode(x)` decodes a percent encoded string.

### Time helpers

- `stdlib.time.nowISO()` returns the current time in ISO 8601 format.
- `stdlib.time.format(layout)` formats the current time with the given layout string.

### JSON helpers

- `stdlib.json.file(path)` reads and parses JSON using the request base directory (only when file access is enabled).
- `stdlib.json.parse(text)` parses a JSON string into RestermScript values.
- `stdlib.json.stringify(value[, indent])` converts a value to JSON text. `indent` can be a string or a number (0-32).
- `stdlib.json.get(value[, path])` returns the value at a dot or `[index]` path (optional leading `$`) and returns null when missing.

### Text helpers

- `stdlib.text.lower(s)` returns a lowercased string.
- `stdlib.text.upper(s)` returns an uppercased string.
- `stdlib.text.trim(s)` trims leading and trailing whitespace.
- `stdlib.text.split(s, sep)` splits a string into a list.
- `stdlib.text.join(list, sep)` joins list items with a separator (items may be strings, numbers, or bools).
- `stdlib.text.replace(s, old, new)` replaces all occurrences of `old` with `new`.
- `stdlib.text.startsWith(s, prefix)` returns true when a string starts with `prefix`.
- `stdlib.text.endsWith(s, suffix)` returns true when a string ends with `suffix`.

### List helpers

- `stdlib.list.append(list, item)` returns a new list with `item` appended.
- `stdlib.list.concat(a, b)` returns a new list with `b` appended to `a`.
- `stdlib.list.sort(list)` returns a sorted copy (numbers or strings only).

### Dict helpers

- `stdlib.dict.keys(dict)` returns a sorted list of keys.
- `stdlib.dict.values(dict)` returns values ordered by sorted keys.
- `stdlib.dict.items(dict)` returns a list of `{key, value}` entries ordered by key.
- `stdlib.dict.set(dict, key, value)` returns a new dict with `key` set.
- `stdlib.dict.merge(a, b)` returns a new dict with `b` applied over `a`.
- `stdlib.dict.remove(dict, key)` returns a new dict without `key`.

### Math helpers

- `stdlib.math.abs(x)` returns the absolute value.
- `stdlib.math.min(a, b)` returns the smaller value.
- `stdlib.math.max(a, b)` returns the larger value.
- `stdlib.math.clamp(x, min, max)` clamps `x` into the range.
- `stdlib.math.floor(x)` returns the largest integer <= x.
- `stdlib.math.ceil(x)` returns the smallest integer >= x.
- `stdlib.math.round(x)` rounds to the nearest integer (half away from zero).

## Host objects for request evaluation

Resterm exposes host objects when evaluating templates, directives, `@apply`, assertions, and pre-request scripts. In pre-request scripts, `request` and `vars` expose mutation helpers, while everything else is read-only. Lookups in `env` and `vars` are case-insensitive; header lookups are normalized, while query keys and JSON paths are case-sensitive.

### env

`env` provides environment values. You can access values through `env.get("name")`, `env.has("name")`, `env.require("name"[, msg])`, or `env.name`. `require` throws when a value is missing.

### vars

`vars` provides request runtime variables, including globals and workflow overrides. You can access values through `vars.get("key")`, `vars.has("key")`, `vars.require("key"[, msg])`, or `vars.key`. `vars.global` provides global reads and writes in pre-request scripts through `get`, `has`, `require`, `set`, and `delete`.

### request

`request` provides a summary of the current request. It exposes `method`, `url`, `headers`, `header(name)`, and `query`. `headers` contains the first value per header (lowercased keys), while `header(name)` is case-insensitive. `query` returns strings or lists when a key has multiple values. In `@script pre-request lang=rts` blocks, mutation helpers are available, including `request.setMethod`, `request.setURL`, `request.setHeader`, `request.addHeader`, `request.removeHeader`, `request.setQueryParam`, and `request.setBody`. In `@apply`, the request object is read only, so you return a patch dict instead of mutating it.

### last

`last` provides a summary of the most recent response. It exposes `status`, `statusCode`, `statusText`, `url`, `headers`, `header(name)`, `text()`, and `json(path)`. `headers` contains the first value per header, while `header(name)` is case-insensitive. `json(path)` accepts a simple dot and `[index]` path (optional leading `$`) and returns null when a value is missing.

### response

`response` provides a summary of the current response when evaluating `@assert`. It has the same shape as `last`.

### trace

`trace` provides timing and budget information for the most recent response. It includes helpers such as `trace.enabled()`, `trace.durationMs()`, `trace.durationSeconds()`, `trace.durationString()`, `trace.error()`, `trace.started()`, `trace.completed()`, `trace.phases()`, `trace.phaseNames()`, `trace.getPhase("dns")`, `trace.budgets()`, `trace.breaches()`, `trace.withinBudget()`, and `trace.hasBudgets()`.

### stream

`stream` provides streaming metadata for SSE and WebSocket requests. It includes helpers such as `stream.enabled()`, `stream.kind()`, `stream.summary()`, and `stream.events()`. Summary and event shapes depend on the stream type (for SSE: `eventCount`, `byteCount`, `duration`, `reason`; for WebSocket: `sentCount`, `receivedCount`, `duration`, `closedBy`, `closeCode`, `closeReason`).

## Directives and workflows

### @use

```
# @use ./rts/helpers.rts as helpers
```

`@use` is valid at file or request scope, and it requires an alias.

### @apply

```
# @apply {headers: {"Authorization": "Bearer " + vars.get("auth.token")}}
```

`@apply` is a request scoped directive and you can use it multiple times in a request. Each apply expression is evaluated in order before pre-request scripts. The expression must return a dict patch with specific keys.

- `method` expects a string and replaces the HTTP method, and Resterm uppercases it.
- `url` expects a string and replaces the request URL.
- `headers` expects a dict where values are strings, numbers, bools, or lists of those; null deletes a header.
- `query` expects a dict where values are strings, numbers, or bools; null deletes the key.
- `body` accepts any value. Strings are used as is, and other values are converted with `str()`.
- `vars` expects a dict and sets request scope variables for this run (values are strings, numbers, or bools).

### @when and @skip-if

```
# @when vars.has("auth.token")
# @skip-if env.mode == "dry-run"
```

These directives are evaluated before pre-request scripts. If the condition is false, the request is skipped and a reason is reported.

### @assert

```
# @assert response.statusCode == 200
# @assert contains(response.header("Content-Type"), "json")
```

Each expression is evaluated and truthy means pass. Use `response` for the current request response.

### @if, @elif, and @else

These directives are used in workflows to branch steps.

```
# @if last.statusCode == 200 run=StepOK
# @elif last.statusCode == 401 run=StepRefresh
# @else fail="unexpected status"
```

### @switch, @case, and @default

```
# @switch last.statusCode
# @case 200 run=StepOK
# @case 401 run=StepRefresh
# @default fail="unexpected status"
```

### @for-each

```
# @for-each json.file("_data/users.json") as user
```

The expression must evaluate to a list. It introduces a loop variable that you can use in RestermScript expressions. In workflows, it also sets `vars.workflow.<name>` and `vars.request.<name>` for legacy templates.

## Limits and safety

RestermScript enforces hard limits to prevent runaway scripts and keep the UI responsive. These limits include maximum steps per evaluation, maximum call depth, maximum string size, maximum list size, maximum dict size, and an optional timeout. When a limit is exceeded, evaluation fails with a detailed error.

## Common patterns

### Guarded requests

```
# @when vars.has("auth.token")
GET {{base_url}}/bearer
Authorization: {{= "Bearer " + vars.get("auth.token") }}
```

### Controlled branching in workflows

```
# @switch last.statusCode
# @case 401 run=Refresh
# @case 200 run=Upsert
# @default fail="unexpected status"
```

### Reusable module logic

```rts
export fn label(user) {
  return (user.name ?? "unknown") + " <" + (user.email ?? "n/a") + ">"
}
```

```http
# @use ./rts/users.rts as users
X-User: {{= users.label(user) }}
```

## Design constraints and why they exist

RestermScript prioritizes predictable evaluation and safe execution. It does not allow file writes or network access, and file reads are limited to `json.file` when enabled. It does not allow member assignment because it reduces side effects and simplifies the interpreter. It requires alias-only imports because that avoids name collisions and keeps modules explicit. It keeps host objects read-only in most contexts because request evaluation should remain declarative. It sorts dict keys during `range` to keep iteration order deterministic across runs.

If you need full scripting or side effects, use JavaScript `@script` blocks. For everything else, RestermScript is the safer and more readable choice.
