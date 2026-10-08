# Language

## Comments

`#` starts a comment that runs to the end of the line. It can appear after whitespace or code.

## Blocks and statement endings

Blocks use `{ ... }` to group statements. A newline ends a statement when the preceding token can finish it. Newlines inside `()` and `[]` are ignored. You can also end a statement with a semicolon; the examples in this guide use newlines.

## Identifiers and keywords

Identifiers start with a letter or `_` and can contain letters, digits, and `_` characters. Keywords are reserved and cannot be used as identifiers.

Keywords:

```text
export module fn let const if elif else switch case default try return for break continue range
true false null and or not
```

A reserved word cannot be a bare name anywhere, which includes dict keys and field access. If your data has a key with the same name as a keyword, use the quoted form:

```rts
let cfg = {"default": 1}
let v = cfg["default"]
```

## Literals

```text
null
true / false
123  3.14
"string"  'string'
[1, 2, 3]
{a: 1, "b": 2}
```

String escapes include `\n`, `\r`, `\t`, `\\`, `\"`, and `\'`. Dict keys in literals are identifiers or quoted strings, and dict keys are always strings at runtime.

## Operators by precedence

Listed from tightest to loosest. Each level binds more tightly than the one below it.

- Postfix: function calls, indexing, and member access.
- Unary: `not` or `!`, `try`, and unary `-`.
- Multiplicative: `*`, `/`, and `%`.
- Additive: `+` and `-`.
- Comparison: `<`, `<=`, `>`, `>=`, `in`, and `not in`.
- Equality: `==` and `!=`.
- Logical AND: `and` or `&&`.
- Logical OR: `or` or `||`.
- Coalescing: `??` returns the right side when the left side is null.
- Ternary: `cond ? a : b` selects between two values.

`??` sits near the bottom, so it binds looser than arithmetic, comparison, and both logical operators. `a ?? b + c` means `a ?? (b + c)`, and `a ?? b or c` means `a ?? (b or c)`. Add parentheses when you want the other grouping.

`+` adds numbers or concatenates strings. Non-numeric values are converted to strings with `str()`. Ordering comparisons only work for numbers or strings, and equality only works for primitive types.

## Membership operators

`value in container` is the infix form of `contains(container, value)`. `value not in container` applies the same membership test and negates its result:

```rts
response.statusCode in [200, 201, 204]
"json" in response.header("Content-Type")
"request_id" in response.json()
response.statusCode not in [400, 404, 500]
```

The container determines how membership works:

- A list compares each item to the value with RestermScript equality. Kinds must match, primitive values compare by value, and lists, dicts, functions, and objects are never deeply equal.
- A string converts the value with `str()` and tests for a substring. The empty string is contained in every string.
- A dict converts the value with `str()` and checks for an exact, case-sensitive key. `null` converts to the empty string, so `null in dict` asks for a `""` key.
- Any other container is an evaluation error.

Host bindings such as `response`, `vars`, and `env` are objects, not containers, so using `in` on them is an error. Use the accessor each one already provides: `vars.has("token")` rather than `"token" in vars`, and `"request_id" in response.json()` rather than `"request_id" in response`.

`not in` is one comparison operator. Because unary `not` binds more tightly than every binary operator, write `value not in container`, not `not value in container`, when you want to negate membership. Like the ordering operators, membership is left-associative. Comparisons are not rewritten into chained tests.

`in` is contextual rather than reserved. It still works as a binding, function or module name, member, and dict key when it is not between two expressions:

```rts
let in = {in: true}
in.in
```

In `.rts` modules and `@rts` blocks, a line may end after either membership operator:

```rts
let allowed = code in
  [200, 201, 204]
```

Directives in request files keep their delimiter-based multiline rule. Open a group when a directive needs to span lines:

```http
# @assert (
#   response.statusCode in
#   [200, 201, 204]
# )
```

## Logical operators

RestermScript supports word and symbolic forms for its logical operators: `and` or `&&`, `or` or `||`, and `not` or `!`. Each pair is interchangeable and has the same precedence and short-circuit behavior:

```rts
if r.ok && !r.retry { return r.value }
if r.ok and not r.retry { return r.value }
```

Logical AND and OR always return a bool, not one of their operands. For example, `1 && 2` evaluates to `true`, not `2`.

Like `not`, `!` binds more tightly than any binary operator, so `!a == b` is parsed as `(!a) == b`. To negate the equality expression instead, write `!(a == b)` or `a != b`.

When a line ends with `&&` or `||`, the expression continues on the next line:

```rts
let ready = r.ok &&
  r.value.count > 0
```

RestermScript does not have bitwise operators, so a single `&` or `|` is a parse error.

## Fallback values with ??

Use `??` to supply a fallback. It returns the left side unless it is null. It is also lazy, so the right side is only evaluated when the left side is null.

```rts
let token = vars.get("auth.token") ?? env.get("auth.token")
let label = candidate ?? "unknown"
```

Because of this, a fallback that is slow or fails costs nothing when it is not needed:

```rts
"ok" ?? fail("boom")   # "ok", fail is never called
```

`??` reacts to null only. `false`, `0`, `""`, the empty list, and the empty dict are real values and pass straight through:

```rts
0 ?? 5      # 0
"" ?? "x"   # ""
```

To replace any falsy value, use the ternary instead:

```rts
let value = candidate ? candidate : "something"
```

`??` does not rescue an undefined name. `missingName ?? "fallback"` is still an error, which keeps typos visible. Optional lookups return null explicitly instead, so `vars.get("missing") ?? "fallback"` works.

### Migrating from default()

`default(a, b)`, `rts.default(a, b)`, and `stdlib.default(a, b)` were removed, and `default` became a reserved word. Replace every call with `(a ?? b)`:

```rts
default(vars.get("token"), "anon")     # removed
(vars.get("token") ?? "anon")          # replacement
```

Keep the parentheses. A call is one tight unit, but `??` binds looser than every operator except the ternary. Without the parentheses, the expression regroups whenever the call was part of a larger one:

```rts
default(a, b) + c   # old, means (a ?? b) + c
a ?? b + c          # wrong, parses as a ?? (b + c)
(a ?? b) + c        # right
```

The parentheses are only redundant when the call was the entire expression, as in the `vars.get` example above.

The replacement is not only shorter. `default(a, b)` was an ordinary call, so `b` was evaluated before the call ran, whether or not `a` was null. `??` evaluates `b` only when `a` is null. If a fallback did real work, that work now happens only when it is needed:

```rts
default(a, fail("missing"))   # always failed
a ?? fail("missing")          # fails only when a is null
```

Check fallbacks that call `uuid()`, mutate `vars`, or fail. Migrating them changes when they run, not only how they are written.

Because `default` is reserved, these forms are now parse errors: `let default = 1`, `fn default() {}`, `{default: 1}`, and `value.default`. If your data has a `default` key, use `{"default": 1}` and `value["default"]`.

## Error handling with try

```rts
try expr
```

The `try` operator evaluates its expression and returns an object with `ok`, `value`, and `error` fields. `ok` is true on success and false on error. `value` holds the result on success and is null on error. `error` is a single-line error string on failure and null on success. It does not catch hard aborts such as step limits, timeouts, or cancellations. You can use `try expr` directly in conditionals, but checking `r.ok` is often clearer.

Example:

```rts
let r = try json.file("_data/users.json")
if not r.ok { return [] }
return r.value
```

Use in `.http` expressions and directives:

```http
# @when try json.file("_data/flags.json")
# @for-each ((try json.file("_data/users.json")).value ?? []) as user
# @assert try response.json("data")

Authorization: Bearer {{= (try last.json("auth.token")).value ?? "" }}
```

This pattern is most useful for optional files, optional JSON bodies, or helper calls that may fail.

## Types and truthiness

RTS has these runtime types:

- Null represents the absence of a value.
- Bool represents true or false.
- Number uses float64 for numeric values.
- String stores UTF-8 text.
- List stores ordered values.
- Dict stores key-value pairs.
- Function represents a callable value.
- Object represents host objects provided by Resterm.

Null, false, zero, the empty string, the empty list, and the empty dict are false. All other values are true unless a host object defines custom truthiness (for example, `try` results are truthy only when `ok` is true).

## Indexing and member access

List indexing uses numeric indices such as `list[0]`, and out-of-range indexes return null. Dict access uses `dict["key"]` or `dict.key`, and missing keys return null. Objects support member access. Whether they support indexing depends on the object.

## Keys and names

Dictionary and query keys are exact strings. Case and whitespace are preserved, including empty query keys. The `rts.dict` helpers behave like `dict[key]`, so `Token`, `token`, and ` token ` are separate keys.

Names used by `env`, `vars`, and request headers have different rules. Resterm makes `env` and `vars` names case-insensitive and ignores surrounding whitespace. Header names are case-insensitive HTTP field names. A name your script supplies is a value, so whitespace around it is rejected instead of trimmed. The header block of a request file is syntax rather than a value, so the parser trims around the colon and then applies the same rule to what is left.

Host maps are validated before evaluation. Blank `env` or `vars` names, and two names with the same identity, are errors, so the result never depends on map order. Header blocks also reject invalid field names and two forms of the same name.

What happens to a name that breaks these rules depends on where it came from. A name your script writes is reported as an error. That includes a header name that is not an HTTP field name, since no request could ever have it:

```rts
request.header("X Token")             # error, not an HTTP field name
headers.get({"X-Ok": "yes"}, "X Tok") # error, same rule
headers.set(h, " X-Token ", "1")      # error, whitespace is not trimmed
```

A malformed host map or header block fails as a whole. Helpers report invalid entries rather than silently dropping them, so the result does not depend on which entry was visited first:

```rts
env.get("   ")                                      # error
headers.get({"X Token": "a", "X-Ok": "yes"}, "X-Ok") # error
```

Header names are checked when the file is parsed, so a header whose name is not an HTTP field name is reported against its line before anything runs. Runtime construction and dispatch also reject it. Invalid names never appear in `request.headers`.
