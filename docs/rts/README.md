# RestermScript

RestermScript (RTS) is the language used to calculate template values, check responses, and control which requests run. You can write expressions directly in request files or share functions through `.rts` modules. [JavaScript hooks](../scripting.md) are also available.

## Why this even exists

Most request logic needs a few calculations or checks rather than a full script. RTS keeps that logic in the request file, with a small syntax and limits on how much work an expression can do.

Expressions stop at a fixed step limit. They cannot make network calls, write files, evaluate arbitrary code, or access system APIs. File reads are limited to `json.file` when file access is enabled. If an expression fails, the error includes the file, line, column, and call stack.

## When to use it

Use RestermScript for calculations, conditions, and checks that belong with a request:

- Template values such as `{{= expr }}` are a good fit when you want computed headers, URLs, or JSON bodies.
- Request and workflow control directives such as `@when`, `@skip-if`, `@if`, `@switch`, and `@for-each` can be driven by RestermScript expressions.
- Use `@assert` for a single response check or `@rts test` for checks that need loops or several statements.
- Reusable `.rts` modules imported with `@use` let you share logic across requests without bringing in JavaScript.

Use JavaScript when you already have hooks you want to keep or need something RestermScript cannot do, such as saving a response body to a file.

## Where it runs

1) Templates

```http
Authorization: Bearer {{= vars.get("auth.token") ?? env.get("auth.token") }}
```

Templates evaluate expressions and insert the results as strings into request fields. Template expressions are read-only.

2) Directives

```http
# @when env.has("feature")
# @assert response.statusCode == 200
```

Directives use expressions to decide whether a request runs or an assertion passes. These expressions are read-only and cannot change request state.

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

An apply patch is an expression that returns a dict describing changes to the outgoing request. Resterm applies those changes before pre-request scripts run. The expression itself sees read-only `request` and `vars` objects.

5) Pre-request scripts

```http
# @rts pre-request
```

Pre-request blocks can change the outgoing request and variables. They run before JavaScript pre-request blocks. `# @script pre-request lang=rts` is the longer form. Use `@assert` or a test block for response checks.

6) Test blocks

```http
# @rts test "has items"
> if len(response.json().items) == 0 {
>   t.error("no items")
> }
```

Each block reports one test result after the response arrives. `t.error` records a failure; `t.fatal` also stops the block. Test blocks cannot change the request or variables. See [@rts](directives.md#rts).
