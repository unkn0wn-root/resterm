# Modules and exports

Modules are `.rts` files that you import with `@use`.

- `module <name>` declares the module name (required when importing without `as`, and it must be the first statement).
- `export` exposes a name from a module.
- `@use ./path.rts` or `@use ./path.rts as alias` imports a module into its request, or into the whole file when it comes before the first `###` line.
- If you omit `as`, the module name becomes the alias.
- Modules are cached, so top-level mutable state can persist across runs.

Example:

```rts
# helpers.rts
module helpers
export fn authHeader(token) {
  return token ? "Bearer " + token : ""
}
```

```http
# @use ./rts/helpers.rts
Authorization: {{= helpers.authHeader(vars.get("auth.token")) }}
```

Modules have access to `rts` and, when the host provides it, `request`. A module cannot declare these names or `stdlib`, but it can declare any other name, including a standard library name such as `json`. Pass other host objects (`env`, `vars`, `last`, `response`, `trace`, `stream`, or a test block's `t`) as function arguments. These objects expire when the block or expression passing them returns. `stdlib` is a deprecated alias for `rts`.
