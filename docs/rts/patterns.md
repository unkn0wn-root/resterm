# Patterns and limits

## Guarded requests

```http
# @when vars.has("auth.token")
GET {{base_url}}/bearer
Authorization: {{= "Bearer " + vars.get("auth.token") }}
```

## Reusable module logic

```rts
module users
export fn label(user) {
  return (user.name ?? "unknown") + " <" + (user.email ?? "n/a") + ">"
}
```

```http
# @use ./rts/users.rts
X-User: {{= users.label(user) }}
```

## Limits and safety

RestermScript enforces hard limits to prevent runaway scripts and keep the UI responsive. When a limit is exceeded, evaluation fails with a detailed error.

| Limit | Value |
| --- | --- |
| Steps per evaluation | 10,000 |
| Call depth | 64 |
| String size | 65,536 bytes |
| List size | 2,000 items |
| Dict size | 2,000 entries |

## Design constraints and why they exist

RestermScript prioritizes predictable evaluation and safe execution. It does not allow file writes or network access, and file reads are limited to `json.file` when enabled. It does not allow member assignment because it reduces side effects and simplifies the interpreter. It requires an explicit alias or module name to avoid name collisions and keep imports explicit. It keeps host objects read-only in most contexts because request evaluation should remain declarative. It sorts dict keys during `range` to keep iteration order deterministic across runs.

If you need full scripting or side effects, use JavaScript `@script` blocks. For everything else, RestermScript is the safer and more readable choice.
