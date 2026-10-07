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

`@apply` is a request-scoped directive, and you can use it more than once in a request. Each apply expression is evaluated in order before pre-request scripts. The expression must return a dict patch with specific keys.

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

In a workflow, put `@when` or `@skip-if` above a `@step` to gate that step. See [Workflows](../workflows.md).

## @assert

```http
# @assert response.statusCode == 200
# @assert "json" in response.header("Content-Type")
# @assert response.statusCode == 201 => "user was not created"
```

Each expression is evaluated, and a truthy result passes. Use `response` for the response to the current request. Add `=> "message"` after the expression to show your own message with the result.

## @if, @elif, and @else

These directives are used in workflows to branch steps. Outside an active workflow, they are ignored and the parser shows a warning. Use `@when` or `@skip-if` to gate an ordinary request.

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
