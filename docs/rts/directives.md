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

`@apply` is a request scoped directive and you can use it multiple times in a request. Each apply expression is evaluated in order before pre-request scripts. The expression must return a dict patch with specific keys.

Header names in a patch follow the same rule as `request.setHeader`: they must be HTTP field names, whitespace is not trimmed, and a patch naming one header twice is an error rather than a choice made by map order. See [Keys and names](language.md#keys-and-names).

You can also reference reusable named patches with `use=`. Comma-separated `use=` entries run left-to-right inside the same `@apply` line.

## @patch

```http
# @patch file jsonApi {headers: {"Accept":"application/json","Content-Type":"application/json"}}
# @patch global authProd {auth: {type:"oauth2", cache_key:"myapi"}}
```

`@patch` defines reusable patch expressions for `@apply use=...`.

- Scope must be `file` or `global`.
- Resolution for `@apply use=name` is file scope first, then global scope.
- Patch names are case-insensitive when resolving.

- `method` expects a string and replaces the HTTP method, and Resterm uppercases it.
- `url` expects a string and replaces the request URL.
- `headers` expects a dict where values are strings, numbers, bools, or lists of those; null deletes a header.
- `query` expects a dict where values are strings, numbers, or bools; null deletes the key.
- `body` accepts any value. Strings are used as is, and other values are converted with `str()`.
- `auth` expects a dict with `type` plus optional params. Use `null` to clear auth for that run.
- `settings` expects a dict where values are strings, numbers, or bools; null deletes a setting key.
- `vars` expects a dict and sets request scope variables for this run (values are strings, numbers, or bools).

## @when and @skip-if

```http
# @when vars.has("auth.token")
# @skip-if env.mode == "dry-run"
```

These directives are evaluated before pre-request scripts. If the condition is false, the request is skipped and a reason is reported.

## @assert

```http
# @assert response.statusCode == 200
# @assert "json" in response.header("Content-Type")
```

Each expression is evaluated and truthy means pass. Use `response` for the current request response.

## @if, @elif, and @else

These directives are used in workflows to branch steps. Outside an active workflow they are ignored with a parser warning; use `@when` or `@skip-if` to gate an ordinary request.

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

These directives route workflow steps and are not the `switch` statement. They share the same equality relation, but each `@case` names a step to run instead of holding a statement list.

## @for-each

```http
# @for-each json.file("_data/users.json") as user
```

The expression must evaluate to a list. It introduces a loop variable that you can use in RestermScript expressions. In workflows, it also sets `vars.workflow.<name>` and `vars.request.<name>` for legacy templates.

The loop variable is a local, so it shadows any standard library, host object, or `@use` alias of the same name for the whole request, including `@rts pre-request` blocks. JavaScript pre-request blocks do not see it as a typed value and continue to read `vars.request.<name>`. See [Name precedence](host-objects.md#name-precedence).
