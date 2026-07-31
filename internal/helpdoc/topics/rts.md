# RestermScript

RestermScript powers `{{= ... }}` template expressions, directive logic, reusable modules, patches, and assertions.

```http
# @when vars.has("token")
# @apply {headers: {"X-Test": "1"}}
# @assert response.statusCode == 200
```

`@when` and `@skip-if` gate a request. `@if` and `@switch` branch workflow steps. Use `@use` to import `.rts` modules. Run `:docs rts` for the language reference.
