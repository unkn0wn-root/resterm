# RestermScript

Use RestermScript for `{{= ... }}` expressions, request conditions, modules, patches, and response checks.

```http
# @when vars.has("token")
# @apply {headers: {"X-Test": "1"}}
# @assert response.statusCode == 200
# @rts test "has items"
> if len(response.json().items) == 0 {
>   t.error("no items")
> }
```

`@when` and `@skip-if` decide whether a request runs. `@if` and `@switch` select workflow steps. In `@rts test` blocks, `t.error` records a failure and continues; `t.fatal` stops the block. Import `.rts` modules with `@use`. Run `:docs rts` for the language reference.
