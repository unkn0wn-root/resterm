# Directives

## @use

```http
# @use ./rts/helpers.rts
# @use ./rts/helpers.rts as helpers
```

`@use` is valid at file or request scope. If you omit `as`, the module name declared with `module <name>` becomes the alias.

## @apply

```http
# @apply {headers: {"Authorization": "Bearer " + vars.get("auth.token")}}
# @apply use=jsonApi,use=authProd
```

Use `@apply` on a request to change its fields before pre-request scripts run. You can add more than one; Resterm evaluates them in order. Each expression must return a dict with the patch keys listed under `@patch` below.

Header names in a patch follow the same rule as `request.setHeader`: they must be HTTP field names, whitespace is not trimmed, and a patch that names one header twice is an error, so the result never depends on map order. See [Keys and names](language.md#keys-and-names).

You can also reference reusable named patches with `use=`. Comma-separated `use=` entries run left to right inside the same `@apply` line.

## @patch

```http
# @patch file jsonApi {headers: {"Accept":"application/json","Content-Type":"application/json"}}
# @patch global authProd {auth: {type:"oauth2", cache_key:"myapi"}}
```

`@patch` defines reusable patch expressions for `@apply use=...`.

- Scope must be `file` or `global`.
- Resolution for `@apply use=name` is file scope first, then global scope.
- Patch names are case-insensitive when resolving.

A patch can contain these keys:

- `method` expects a string and replaces the HTTP method. Resterm uppercases it.
- `url` expects a string and replaces the request URL.
- `headers` expects a dict where values are strings, numbers, bools, or lists of those. Null deletes a header.
- `query` expects a dict where values are strings, numbers, or bools. Null deletes the key.
- `body` accepts any value. Strings are used as-is, and other values are converted with `str()`.
- `auth` expects a dict with `type` plus optional params. Use `null` to clear auth for that run.
- `settings` expects a dict where values are strings, numbers, or bools. Null deletes a setting key.
- `vars` expects a dict and sets request-scope variables for this run (values are strings, numbers, or bools).

## @when and @skip-if

```http
# @when vars.has("auth.token")
# @skip-if env.mode == "dry-run"
```

These directives are evaluated before pre-request scripts. If a `@when` condition is false or a `@skip-if` condition is true, the request is skipped and Resterm reports why.

In a workflow, put `@when` or `@skip-if` above a `@step` to decide whether that step runs. When the step has its own `@for-each`, the condition is checked for each item instead. See [Workflows](../workflows.md).

## @assert

```http
# @assert response.statusCode == 200
# @assert "json" in response.header("Content-Type")
# @assert response.statusCode == 201 => "user was not created"
```

Each expression is evaluated, and a truthy result passes. Use `response` for the response to the current request. Add `=> "message"` after the expression to show your own message with the result.

## @rts

```http
# @use ./validators.rts
# @rts pre-request
> request.setHeader("X-Request-Id", uuid())
# @rts test "orders are valid"
> for let _, msg range validators.orders(response.json().orders) {
>   t.error(msg)
> }
```

Use `@rts pre-request` to change the outgoing request and `@rts test` to check the response. `# @script pre-request lang=rts` and `# @script test lang=rts` are equivalent.

Add a name after `test`; quote it if it contains spaces. Extra unquoted words are an error. Unnamed blocks, including `@script test lang=rts`, appear as `@rts test` in the results.

Prefix each code line with `>`. A blank line or comment without `>` ends the block; later code starts another block with the same name and a new scope. Use `> #` for comments and `>` for blank lines within a block.

Use `> {%` and `%}` on separate lines to keep comments and blank lines in a block. Inside the braces, `>` is optional:

```http
# @use ./validators.rts
# @rts test "orders are valid"
> {%
# Validate orders
let orders = response.json().orders

for let _, msg range validators.orders(orders) {
  t.error(msg)
}
%}
```

One-line blocks also work: `> {% request.setHeader("X-Debug", "1") %}` in a pre-request block. To read code from a file, use `> < ./checks.rts` outside the `{% ... %}` block. The file contains plain code without `>`.

Response checks run after captures: `@assert` first, then RTS test blocks in file order, then JavaScript tests. An assertion failure or error does not skip the test blocks. This applies to HTTP, gRPC, SSE, and WebSocket requests with `@ws` steps.

Test blocks can read `response`, `last`, `stream`, `trace`, `env`, `vars`, `request`, imported modules, loop values, and captures. Calls to `vars.set`, `vars.global.set`, or request mutation helpers fail. Use `@capture` to save response values, including module results: `# @capture file token = auth.pick(response.json())`.

Report failures with [`t.error` and `t.fatal`](host-objects.md#t). Runtime errors, including `fail()`, also fail the block and include the file and line. Earlier failures are kept, and later blocks still run. `resterm run` exits with code 1 for test failures or 24 for script errors. See [Exit codes](../cli/run.md#exit-codes).

## @if, @elif, and @else

Use these directives in workflows to choose a step based on a condition. Outside an active workflow, they are ignored and the parser shows a warning. Use `@when` or `@skip-if` to decide whether an ordinary request runs.

```http
# @if last.statusCode == 200 run=StepOK
# @elif last.statusCode == 401 run=StepRefresh
# @else fail="unexpected status"
```

## @switch, @case, and @default

```http
# @switch last.statusCode
# @case 200 run=StepOK
# @case 401 run=StepRefresh
# @default fail="unexpected status"
```

These directives route workflow steps and are not the `switch` statement. They use the same equality rules, but each `@case` names a step to run instead of holding a statement list.

## @for-each

```http
# @for-each json.file("_data/users.json") as user
```

The expression must evaluate to a list. It introduces a loop variable that you can use in RestermScript expressions. In workflows, it also sets `vars.workflow.<name>` and `vars.request.<name>` for legacy templates. `@for-each user in json.file("_data/users.json")` is the same loop with the variable first. In a workflow, `@for-each` above a `@step` repeats that step.

The loop variable is a local, so it shadows any standard library, host object, or `@use` alias of the same name for the whole request, including `@rts pre-request` blocks. JavaScript pre-request blocks do not see it as a typed value and still read `vars.request.<name>`. See [Name precedence](host-objects.md#name-precedence).
