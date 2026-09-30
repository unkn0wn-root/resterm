# Modules and exports

Modules are `.rts` files and they are imported with `@use`.

- `module <name>` declares the module name (required when importing without `as`, and it must be the first statement).
- `export` exposes a name from a module.
- `@use ./path.rts` or `@use ./path.rts as alias` imports a module into a request or file.
- If you omit `as`, the module name becomes the alias.
- Modules are cached, so top level mutable state can persist across runs.

Example:

```rts
// helpers.rts
module helpers
export fn authHeader(token) {
  return token ? "Bearer " + token : ""
}
```

```http
# @use ./rts/helpers.rts
Authorization: {{= helpers.authHeader(vars.get("auth.token")) }}
```

Modules run with `rts` only. The `request` object is available when the host provides it, but `env`, `vars`, `last`, `response`, `trace`, and `stream` are not. Pass values in as arguments when you need extra context. `stdlib` remains available as a deprecated alias.
