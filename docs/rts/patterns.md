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

## Response checks with a module

Return validation messages from a module and report them with `t.error`. The module can also be called from `@assert` or `@capture`:

```rts
module validators

export fn orders(list) {
  let msgs = []
  for let i, o range list {
    let msg = order(o)
    if msg != null {
      msgs = rts.list.append(msgs, "order " + str(i) + ": " + msg)
    }
  }
  return msgs
}

fn order(o) {
  if o.total < 0 {
    return "total " + str(o.total) + " is negative"
  }
  if not o.items {
    return "no items"
  }
  return null
}
```

```http
# @use ./rts/validators.rts

### Fetch orders
# @rts test "orders are valid"
> let body = try response.json()
> if not body.ok {
>   t.fatal("body is not JSON:", body.error)
> }
> for let _, msg range validators.orders(body.value.orders) {
>   t.error(msg)
> }
GET {{base_url}}/orders
```

## Limits and safety

RestermScript stops evaluation when any of these limits is exceeded. The error identifies the limit, so a runaway script cannot keep the UI busy indefinitely.

| Limit | Value |
| --- | --- |
| Steps per expression | 10,000 |
| Steps per `@rts` or `lang=rts` block | 100,000 |
| Call depth | 64 |
| String size | 65,536 bytes |
| List size | 2,000 items |
| Dict size | 2,000 entries |

Templates and directives such as `@assert`, `@capture`, and `@apply` use the expression limit. Module functions use the caller's budget. Module initialization has a separate 10,000-step budget.

## Design constraints and why they exist

RTS does not allow file writes or network calls. It can read files only through `json.file`, when enabled. Use JavaScript `@script` blocks when you need fuller scripting support or side effects.

Other restrictions make request logic easier to follow. You cannot assign to an object member, which limits side effects and keeps the interpreter simple. Imports need an explicit alias or module name so names do not collide. Only pre-request blocks can change the request and variables. Test blocks are read-only; use `@capture` to save response values. Dict keys are sorted during `range`, so the same dict is visited in the same order on every run.
