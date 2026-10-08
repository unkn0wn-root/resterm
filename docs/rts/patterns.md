# Patterns and limits

## Guarded requests

Skip a request when a required value is missing. Here, the request runs only if an auth token is available:

```http
# @when vars.has("auth.token")
GET {{base_url}}/bearer
Authorization: {{= "Bearer " + vars.get("auth.token") }}
```

## Reusable module logic

Put a function in a module when several requests need it. This function builds a user label with fallback values for missing fields:

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

RestermScript stops evaluation when any of these limits is exceeded. The error identifies the limit, so a runaway script cannot keep the UI busy indefinitely.

| Limit | Value |
| --- | --- |
| Steps per evaluation | 10,000 |
| Call depth | 64 |
| String size | 65,536 bytes |
| List size | 2,000 items |
| Dict size | 2,000 entries |

## Design constraints and why they exist

RTS does not allow file writes or network calls. It can read files only through `json.file`, when enabled. Use JavaScript `@script` blocks when you need fuller scripting support or side effects.

Other restrictions make request logic easier to follow. You cannot assign to an object member, which limits side effects and keeps the interpreter simple. Imports need an explicit alias or module name so names do not collide. Most host objects are read-only during request evaluation; pre-request blocks can change the request and variables. Dict keys are sorted during `range`, so the same dict is visited in the same order on every run.
