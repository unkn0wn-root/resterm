# RestermScript

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

```http
Authorization: Bearer {{= vars.get("auth.token") ?? env.get("auth.token") }}
```

Templates evaluate expressions and insert their string results into request fields. They are read only and should not cause side effects.

2) Directives

```http
# @when env.has("feature")
# @assert response.statusCode == 200
```

Directives evaluate expressions to decide whether a request runs or whether an assertion passes. They are read only and should not mutate request state.

3) Modules

```http
# @use ./rts/helpers.rts
# @use ./rts/helpers.rts as helpers
```

Modules are compiled once and expose only exported names through the alias (explicit or module name). [Modules and exports](modules.md) covers what a module can see.

4) Apply patches

```http
# @apply {headers: {"X-Test": "1"}}
```

Apply patches evaluate a single RestermScript expression that returns a patch dict and applies it to the outgoing request. They run before pre-request scripts and use read-only `request` and `vars` objects.

5) Pre request scripts

```http
# @rts pre-request
```

Pre-request scripts run full RestermScript blocks and can mutate the outgoing request and variables. They run before JavaScript pre-request blocks. The full `# @script pre-request lang=rts` form remains supported. Use `@assert` for RestermScript response checks.
