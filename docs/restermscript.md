# RestermScript Technical Reference

RestermScript (or RTS which you will see quite often throughout the docs) is Resterm's built in expression language for templates, directives, and reusable modules. It is designed to be small, bounded, and easy to review inside request files. JavaScript via Goja is still available, but RestermScript is the preferred option when you want predictable behavior, clear errors, and safe execution.

## Why this even exists

- RTS is bounded and predictable because expressions run with strict step limits, cannot perform network operations or file writes, and only read files via `json.file` when file access is enabled.
- RTS is safe because it avoids arbitrary evaluation and does not expose system APIs.
- RTS is clear because the syntax is small and purpose built for request files.
- RTS is debuggable because errors include file, line, and column information along with a call stack.

## When to use it

Use RestermScript when you need small, safe logic for request evaluation and control flow.

- Template values such as `{{= expr }}` are a good fit when you want computed headers, URLs, or JSON bodies.
- Request and workflow control directives such as `@when`, `@skip-if`, `@if`, `@switch`, and `@for-each` can be driven by RestermScript expressions.
- Assertions using `@assert` are readable and produce clear failures.
- Reusable `.rts` modules imported with `@use` let you share logic across requests without bringing in JavaScript.

Use JavaScript only when you need full language features or when porting existing logic is not worth the rewrite.

## Where it runs

1) Templates

```
Authorization: Bearer {{= vars.get("auth.token") ?? env.get("auth.token") }}
```

Templates evaluate expressions and insert their string results into request fields. They are read only and should not cause side effects.

2) Directives

```
# @when env.has("feature")
# @assert response.statusCode == 200
```

Directives evaluate expressions to decide whether a request runs or whether an assertion passes. They are read only and should not mutate request state.

3) Modules

```
# @use ./rts/helpers.rts
# @use ./rts/helpers.rts as helpers
```

Modules are compiled once and expose only exported names through the alias (explicit or module name). Modules execute with `rts`; `stdlib` remains as a deprecated alias. When the host provides a `request` object it is available (read-only outside pre-request scripts). Modules do not automatically see `env`, `vars`, `last`, `response`, `trace`, or `stream`, so pass values into module functions explicitly.

4) Apply patches

```
# @apply {headers: {"X-Test": "1"}}
```

Apply patches evaluate a single RestermScript expression that returns a patch dict and applies it to the outgoing request. They run before pre-request scripts and use read-only `request` and `vars` objects.

5) Pre request scripts

```
# @rts pre-request
```

Pre-request scripts run full RestermScript blocks and can mutate the outgoing request and variables. They run before JavaScript pre-request blocks. The full `# @script pre-request lang=rts` form remains supported. Use `@assert` for RestermScript response checks.

## Language overview

### Comments

`#` starts a comment that runs to the end of the line. It can appear after whitespace or code.

### Blocks and statement endings

Blocks use `{ ... }` and group statements together. A newline can end a statement when the previous token can finish a statement. Newlines inside `()` and `[]` are ignored. The language also accepts the semicolon token as a statement terminator, but this guide uses newlines for clarity.

### Identifiers and keywords

Identifiers start with a letter or `_` and can contain letters, digits, and `_` characters. The language reserves keywords and they cannot be used as identifiers.

Keywords:

```
export module fn let const if elif else switch case default try return for break continue range
true false null and or not
```

A reserved word cannot be a bare name anywhere, which includes dict keys and field access. Data that carries a key spelled like a keyword uses the quoted forms instead:

```
let cfg = {"default": 1}
let v = cfg["default"]
```

### Literals

```
null
true / false
123  3.14
"string"  'string'
[1, 2, 3]
{a: 1, "b": 2}
```

String escapes include `\n`, `\r`, `\t`, `\\`, `\"`, and `\'`. Dict keys in literals are identifiers or quoted strings, and dict keys are always strings at runtime.

### Operators by precedence

Listed from tightest to loosest. Each level binds more tightly than the one below it.

- Postfix: function calls, indexing, and member access.
- Unary: `not` or `!`, `try`, and unary `-`.
- Multiplicative: `*`, `/`, and `%`.
- Additive: `+` and `-`.
- Comparison: `<`, `<=`, `>`, and `>=`.
- Equality: `==` and `!=`.
- Logical AND: `and` or `&&`.
- Logical OR: `or` or `||`.
- Coalescing: `??` returns the right side when the left side is null.
- Ternary: `cond ? a : b` selects between two values.

`??` sits near the bottom, so it binds looser than arithmetic, comparison, and both logical operators. `a ?? b + c` means `a ?? (b + c)`, and `a ?? b or c` means `a ?? (b or c)`. Parenthesise when you want the other grouping.

`+` adds numbers or concatenates strings. Non numeric values are converted to string using `str()`. Comparisons only work for numbers or strings, and equality only works for primitive types.

### Logical operators

RestermScript supports word and symbolic forms for its logical operators: `and` or `&&`, `or` or `||`, and `not` or `!`. Each pair is interchangeable and has the same precedence and short-circuit behavior:

```
if r.ok && !r.retry { return r.value }
if r.ok and not r.retry { return r.value }
```

Logical AND and OR always return a bool, not one of their operands. For example, `1 && 2` evaluates to `true`, not `2`.

Like `not`, `!` binds more tightly than any binary operator. This means `!a == b` is parsed as `(!a) == b`. To negate the equality expression instead, write `!(a == b)` or simply `a != b`.

When a line ends with `&&` or `||`, the expression continues on the next line:

```
let ready = r.ok &&
  r.value.count > 0
```

RestermScript does not have bitwise operators, so a single `&` or `|` is a parse error.

### Fallback values with ??

`??` is the way to supply a fallback. It returns the left side unless it is null, and it is lazy: the right side is only evaluated when the left side is null.

```
let token = vars.get("auth.token") ?? env.get("auth.token")
let label = candidate ?? "unknown"
```

Laziness means the fallback can be expensive or failing without cost when it is not needed:

```
"ok" ?? fail("boom")   # "ok", fail is never called
```

`??` reacts to null only. `false`, `0`, `""`, the empty list, and the empty dict are real values and pass straight through:

```
0 ?? 5      # 0
"" ?? "x"   # ""
```

When you do want any falsey value replaced, that is a different question and the ternary answers it:

```
let value = candidate ? candidate : "something"
```

`??` does not rescue an undefined name. `missingName ?? "fallback"` is still an error, which keeps typos visible. Optional lookups return null explicitly instead, so `vars.get("missing") ?? "fallback"` works.

#### Migrating from default()

`default(a, b)`, `rts.default(a, b)`, and `stdlib.default(a, b)` were removed, and `default` became a reserved word. Replace every call with `(a ?? b)`:

```
default(vars.get("token"), "anon")     # removed
(vars.get("token") ?? "anon")          # replacement
```

Keep the parentheses. A call is a single tight unit, but `??` binds looser than every operator except the ternary, so dropping them regroups the expression whenever the call was part of a larger one:

```
default(a, b) + c   # old, means (a ?? b) + c
a ?? b + c          # wrong, parses as a ?? (b + c)
(a ?? b) + c        # right
```

The parentheses are only redundant when the call was the entire expression, as in the `vars.get` example above.

The replacement is not only shorter. `default(a, b)` was an ordinary call, so `b` was evaluated before the call ran, whether or not `a` was null. `??` evaluates `b` only when `a` is null. If a fallback did real work, that work now happens only when it is actually needed:

```
default(a, fail("missing"))   # always failed
a ?? fail("missing")          # fails only when a is null
```

Check fallbacks that call `uuid()`, mutate `vars`, or fail. Migrating them changes when they run, not just how they are spelled.

Because `default` is reserved, these spellings are now parse errors: `let default = 1`, `fn default() {}`, `{default: 1}`, and `value.default`. Dict data that genuinely has a `default` key uses `{"default": 1}` and `value["default"]`.

### Error handling with try

```
try expr
```

The `try` operator evaluates its expression and returns an object with `ok`, `value`, and `error` fields. `ok` is true on success and false on error. `value` holds the result on success and is null on error. `error` is a single line error string on failure and null on success. It does not catch hard aborts such as step limits, timeouts, or cancellations. You can use `try expr` directly in conditionals, but checking `r.ok` is often clearer.

Example:

```
let r = try json.file("_data/users.json")
if not r.ok { return [] }
return r.value
```

Use in `.http` expressions and directives:

```
# @when try json.file("_data/flags.json")
# @for-each ((try json.file("_data/users.json")).value ?? []) as user
# @assert try response.json("data")

Authorization: Bearer {{= (try last.json("auth.token")).value ?? "" }}
```

This pattern is most useful for optional files, optional JSON bodies, or helper calls that may fail.

### Types and truthiness

RTS has several runtime types.

- Null represents the absence of a value.
- Bool represents true or false.
- Number uses float64 for numeric values.
- String stores UTF 8 text.
- List stores ordered values.
- Dict stores key value pairs.
- Function represents a callable value.
- Object represents host objects provided by Resterm.

Truthiness follows consistent rules. Null, false, zero, the empty string, the empty list, and the empty dict are false. All other values are true unless a host object defines custom truthiness (for example, `try` results are truthy only when `ok` is true).

### Indexing and member access

List indexing uses numeric indices such as `list[0]`, and out of range accesses return null. Dict access uses `dict["key"]` or `dict.key`, and missing keys return null. Object member access is supported, while indexing depends on the object implementation.

### Keys and names

Dictionary and query keys are exact strings. Case and whitespace are preserved, including empty query keys. The `rts.dict` helpers behave like `dict[key]`, so `Token`, `token`, and ` token ` are separate keys.

Names used by `env`, `vars`, and request headers have different rules. Resterm makes `env` and `vars` names case-insensitive and ignores surrounding whitespace. Header names are case-insensitive HTTP field names. A name your script supplies is a value, so whitespace around it is rejected instead of trimmed. The header block of a request file is syntax rather than a value, so the parser trims around the colon and then holds what is left to the same rule.

Host maps are validated before evaluation. Blank `env` or `vars` names, and two spellings with the same identity, are errors instead of choices made by map order. Header blocks likewise reject invalid field names and equivalent spellings.

What happens to a name a rule does not accept depends on where it came from. A name your script writes is your own word, and a header name that is not an HTTP field name asks a question no request can answer, so it is reported:

```rts
request.header("X Token")             // error, not an HTTP field name
headers.get({"X-Ok": "yes"}, "X Tok") // error, same rule
headers.set(h, " X-Token ", "1")      // error, whitespace is not trimmed
```

A malformed host map or header block fails as one value; helpers never silently discard an entry. This keeps all evaluations deterministic and makes bad input visible at the boundary:

```rts
env.get("   ")                                      // error
headers.get({"X Token": "a", "X-Ok": "yes"}, "X-Ok") // error
```

Header names are checked when the file is parsed, so a header whose name is not an HTTP field name is reported against its line before anything runs. Runtime construction and dispatch also reject it; invalid names are never exposed through `request.headers`.

## Statements

### let and const

```
let name = expr
const name = expr
```

`let` creates a mutable binding and `const` creates an immutable binding. Redeclaring a name in the same scope is an error, while shadowing a name in an inner block is allowed. Assignment requires the name to exist in the current or parent scope.

### Assignment

```
name = expr
```

Assignment only applies to variable names. Member assignment and index assignment are not supported.

### Functions

```
fn add(a, b) {
  return a + b
}
```

Functions close over their lexical environment. Function names are immutable because `fn` defines a constant binding. Function parameters are local variables and can be reassigned.

### Conditionals

```
if cond {
  ...
} elif other {
  ...
} else {
  ...
}
```

Conditionals evaluate each branch in order and execute the first branch whose condition is true. The `else` branch runs only when no earlier condition is true.

### switch

`switch` picks one branch out of many. The tagged form compares a value against cases, and the tagless form replaces a long `if`/`elif` chain.

```
switch response.statusCode {
case 200, 201:
  result = "success"
case 401:
  result = "unauthorized"
default:
  result = "unexpected"
}
```

```
switch {
case score >= 90:
  grade = "A"
case score >= 80:
  grade = "B"
default:
  grade = "C"
}
```

Grammar:

```
SwitchStmt = "switch" [ Expression ] "{" { CaseClause } "}" .
CaseClause = "case" Expression { "," Expression } ":" StatementList
           | "default" ":" StatementList .
```

Rules:

- The tag is evaluated exactly once, before any case.
- Clauses run top to bottom and the expressions within a clause run left to right. Evaluation stops at the first match, so later case expressions never run.
- Only the matching clause runs. There is no fallthrough, implicit or explicit.
- A tagged switch matches with the same equality as `==`. Kinds must match, and only null, bool, number, and string compare by value. Lists and dicts never compare equal, so `switch [1] { case [1]: ... }` falls through to `default`.
- A tagless switch takes the first case expression that is truthy, using the same truth test as `if`. It does not require a bool.
- `case` takes one or more expressions separated by commas. A comma can be followed by a newline, but the colon must stay on the last expression's line.
- A clause body can be empty, in which case a match does nothing.
- At most one `default` is allowed. It can sit anywhere among the cases and runs only when no case matched.
- Every clause is its own scope. `let` and `const` inside a clause do not escape it, while assignment to an outer binding works as usual.
- `break` leaves the nearest enclosing switch or loop. A `break` inside a switch that sits in a loop ends the switch, and the loop continues. Use a label when you need to leave the loop instead.
- `continue` always targets the nearest enclosing loop, including from inside a switch.
- `return`, runtime errors, and hard aborts propagate out of the switch normally.

A `{` right after `switch` always starts the tagless form, so a dict literal tag needs parentheses:

```
switch ({a: 1}).a {
case 1:
  matched = true
}
```

Case expressions are arbitrary runtime expressions, so duplicate cases are not reported at parse time. The first one written wins.

`default` is a reserved word, so it is always the clause label and never a name. Use `??` for a fallback value:

```
switch value {
default:
  value = candidate ?? "fallback"
}
```

Switch initializers, type switches, switch expressions that produce a value, and `fallthrough` are not part of the language. A switch can carry a label so that a `break` deeper inside can leave it by name, described under [Labels](#labels).

This statement is separate from the `@switch` workflow directive. `@switch` selects a workflow step in an `.http` file and shares the same equality relation, while `switch` is a statement inside RestermScript code.

### for loops

RTS supports several loop forms.

```
for { ... }
for cond { ... }
for let k, v range expr { ... }
```

The language also supports a three clause loop with init, condition, and post clauses. The clauses are separated by the semicolon token.

Rules for loops are consistent. `continue` is valid only inside loops, and `break` is valid inside loops and switches. Both accept a label to target an enclosing statement by name. `const` is not allowed in loop headers. `for let` introduces loop scoped variables that do not escape the loop block. `for range` without `let` assigns to existing variables.

### range semantics

Range iteration is deterministic and follows clear rules.

- When you range a list, the key is the index and the value is the item.
- When you range a dict, the key is the string key and the value is the item, and keys are sorted to keep output stable.
- When you range a string, the key is the byte index and the value is a single rune string.

Example:

```
for let i, ch range "go" {
  // i is the byte index, ch is "g" and then "o"
}
```

### Labels

A label names a `for` or a `switch` so that a `break` or `continue` deeper inside can target it by name. Without labels there is no way to leave a loop from inside a switch, because a plain `break` stops at the switch.

```
outer: for let i = 0; i < 10; i = i + 1 {
  switch i {
  case 5:
    break outer
  }
}
```

`continue label` resumes the named loop, skipping the rest of every construct in between:

```
outer: for let i, row range rows {
  for let j, value range row {
    switch value {
    case null:
      continue outer
    }
  }
}
```

Grammar:

```
LabeledStmt  = identifier ":" ( ForStmt | SwitchStmt ) .
BreakStmt    = "break" [ identifier ] .
ContinueStmt = "continue" [ identifier ] .
```

Rules:

- A label can decorate only a `for` or a `switch`. There are no labeled blocks, no labels on `if` or `fn`, and no `goto`.
- The unlabeled forms are unchanged. `break` still leaves the nearest switch or loop, and `continue` still resumes the nearest loop.
- `break label` leaves the named statement. `continue label` resumes the named loop and runs that loop's post clause once, skipping the post clause of every loop it passed through.
- The target must lexically enclose the `break` or `continue`, and labels do not cross a function boundary.
- `continue` must name a loop. Naming a switch is an error.
- Labels live in their own namespace, so a label never collides with a variable or function of the same name.
- Two active labels cannot share a name, but a name is free again once its statement ends, so sibling statements can reuse it.
- `_` is not a valid label, and neither is any reserved word.
- The label of a `break` or `continue` has to stay on the same line as the keyword, because a newline there already ends the statement. A label in front of a `for` or `switch` may sit on its own line.

## Modules and exports

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

## Standard library

RTS provides a small standard library that covers common request needs without enabling file writes or network access. It keeps expressions small, readable, and predictable. The standard library is available as `rts`; `stdlib` remains as a deprecated alias. Core helpers and namespaces (`crypto`, `base64`, `url`, `time`, `json`, `headers`, `query`, `encoding`) are also exposed at top level for convenience. `text`, `list`, `dict`, and `math` are available only under `rts`.

### Core helpers

- `rts.fail(msg)` stops evaluation and returns an error message.
- `rts.len(x)` returns the length of a string, list, or dict.
- `rts.contains(haystack, needle)` checks whether a value is contained in a string, list, or dict.
- `rts.match(pattern, text)` applies a regular expression to text and returns true when it matches.
- `rts.str(x)` converts a value to a string, using JSON for lists and dicts.
- `rts.num(x[, def])` converts a value to a number, or returns `def` when conversion fails.
- `rts.int(x[, def])` converts a value to an integer, or returns `def` when conversion fails.
- `rts.bool(x[, def])` converts a value to a bool, or returns `def` when conversion fails.
- `rts.typeof(x)` returns the type name.
- `rts.uuid()` generates a UUID and requires random generation to be enabled.

### Crypto helpers

- `rts.crypto.sha256(text)` returns a hex encoded SHA-256 digest.
- `rts.crypto.hmacSha256(key, text)` returns a hex encoded HMAC-SHA256 digest.

### Encoding and URL helpers

- `rts.base64.encode(x)` encodes a string to base64.
- `rts.base64.decode(x)` decodes a base64 string.
- `rts.encoding.hex.encode(x)` encodes a string to hex.
- `rts.encoding.hex.decode(x)` decodes a hex string.
- `rts.encoding.base64url.encode(x)` encodes a string to base64url (no padding).
- `rts.encoding.base64url.decode(x)` decodes a base64url string.
- `rts.url.encode(x)` percent encodes a string for URL use.
- `rts.url.decode(x)` decodes a percent encoded string.

### Time helpers

- `rts.time.nowISO()` returns the current time in ISO 8601 format.
- `rts.time.nowUnix()` returns the current time as unix seconds.
- `rts.time.nowUnixString()` returns the current time as a decimal unix seconds string.
- `rts.time.nowUnixMs()` returns the current time as unix milliseconds.
- `rts.time.format(layout)` formats the current time with the given layout string.
- `rts.time.parse(layout, value)` parses the time string and returns unix seconds (fractional).
- `rts.time.formatUnix(ts, layout)` formats a unix timestamp with the given layout.
- `rts.time.addUnix(ts, secondsOrDuration)` adds seconds (number) or a duration string to a unix timestamp.
- `rts.time.duration(value)` parses a duration string (including `d` and `w`) and returns seconds.

### JSON helpers

- `rts.json.file(path)` reads and parses JSON using the request base directory (only when file access is enabled).
- `rts.json.parse(text)` parses a JSON string into RestermScript values.
- `rts.json.stringify(value[, indent])` converts a value to JSON text. `indent` can be a string or a number (0-32).
- `rts.json.get(value[, path])` returns the value at a dot or `[index]` path (optional leading `$`) and returns null when missing.
- `rts.json.has(value, path)` returns true when a value exists at the path.

### Text helpers

- `rts.text.lower(s)` returns a lowercased string.
- `rts.text.upper(s)` returns an uppercased string.
- `rts.text.trim(s)` trims leading and trailing whitespace.
- `rts.text.split(s, sep)` splits a string into a list.
- `rts.text.join(list, sep)` joins list items with a separator (items may be strings, numbers, or bools).
- `rts.text.replace(s, old, new)` replaces all occurrences of `old` with `new`.
- `rts.text.startsWith(s, prefix)` returns true when a string starts with `prefix`.
- `rts.text.endsWith(s, suffix)` returns true when a string ends with `suffix`.

### List helpers

- `rts.list.append(list, item)` returns a new list with `item` appended.
- `rts.list.concat(a, b)` returns a new list with `b` appended to `a`.
- `rts.list.sort(list)` returns a sorted copy (numbers or strings only).
- `rts.list.map(list, fn)` returns a new list with `fn(item)` applied to each value.
- `rts.list.filter(list, fn)` returns a new list of values where `fn(item)` is truthy.
- `rts.list.any(list, fn)` returns true if any value makes `fn(item)` truthy.
- `rts.list.all(list, fn)` returns true if all values make `fn(item)` truthy.
- `rts.list.slice(list, start[, end])` returns a slice of the list.
- `rts.list.unique(list)` returns a list of unique primitive values.

### Dict helpers

- `rts.dict.keys(dict)` returns a sorted list of keys.
- `rts.dict.values(dict)` returns values ordered by sorted keys.
- `rts.dict.items(dict)` returns a list of `{key, value}` entries ordered by key.
- `rts.dict.set(dict, key, value)` returns a new dict with `key` set.
- `rts.dict.merge(a, b)` returns a new dict with `b` applied over `a`.
- `rts.dict.remove(dict, key)` returns a new dict without `key`.
- `rts.dict.get(dict, key[, def])` returns `def` or null when missing.
- `rts.dict.has(dict, key)` returns true when a key exists.
- `rts.dict.pick(dict, keys)` returns a dict with the specified keys.
- `rts.dict.omit(dict, keys)` returns a dict without the specified keys.

These helpers use keys exactly as written, like `dict[key]`. See [Keys and names](#keys-and-names).

### Header and query helpers

- `headers.get(h, name)` returns the first value of a header, or null when it is missing.
- `headers.has(h, name)` returns true when a header carries a value.
- `headers.set(h, name, value)` returns a new dict with a string or `list<string>` set, replacing the header regardless of name casing.
- `headers.remove(h, name)` returns a new dict without the header, regardless of name casing.
- `headers.merge(a, b)` returns a new dict with `b` applied over `a`. A null value in `b` removes that header.
- `headers.normalize(h)` returns a new dict with the names lowercased.
- `query.parse(rawQuery)` parses raw query text. It never guesses that its argument is a URL.
- `query.fromURL(url)` parses the query component of a URL.
- `query.encode(query)` encodes a query multimap into a query string.
- `query.merge(url, query)` returns the URL with the parameters applied. Null or an empty list removes a parameter.

Header and query dictionaries use cardinality-based values: `dict<string, string | list<string>>`. One value is a string, multiple values are a list, and zero values are an empty list. The result depends on the number of values rather than the input syntax, so a one-element input list is returned as a string. Helpers do not coerce numbers or booleans into strings. Null is not a stored value; it is accepted only as the removal marker in the patch argument to `headers.merge` and `query.merge`.

Header names are case-insensitive HTTP field names. Two forms of the same header always return an error, because picking one would depend on map order. Every header helper validates the entire input block and the requested name. Returned header names are lowercased, and `headers.get` returns the first value regardless of whether the stored representation is a string or list.

Query helpers keep keys and values exactly as written, including empty keys and whitespace. Encoding preserves the data but may change order and escaping. `query.parse` removes one leading `?` as syntax and treats every other byte as query data. `query.fromURL` and `query.merge` do not trim or repair their URL argument. Use `rts.text.trim` explicitly when that is the behavior you want.

### Math helpers

- `rts.math.abs(x)` returns the absolute value.
- `rts.math.min(a, b)` returns the smaller value.
- `rts.math.max(a, b)` returns the larger value.
- `rts.math.clamp(x, min, max)` clamps `x` into the range.
- `rts.math.floor(x)` returns the largest integer <= x.
- `rts.math.ceil(x)` returns the smallest integer >= x.
- `rts.math.round(x)` rounds to the nearest integer (half away from zero).

## Host objects for request evaluation

Resterm exposes host objects when evaluating templates, directives, `@apply`, assertions, and pre-request scripts. In pre-request scripts, `request` and `vars` expose mutation helpers. Other available objects are read-only. Lookups in `env` and `vars` are case-insensitive and ignore surrounding whitespace. Header lookups are case-insensitive, while query parameters and JSON paths are exact. See [Keys and names](#keys-and-names). The TUI also exposes `mock` during request evaluation. Its helpers work while the workspace mock server is running and return an error when it is stopped.

### Name precedence

Every evaluation binds its names in a fixed order, and a later layer wins:

1. The standard library and the host objects below.
2. Host extensions such as `mock`. These are additive and cannot reuse a name from the first layer.
3. `@use` aliases. These cannot reuse a name from either layer above.
4. Local values, currently the `@for-each` loop variable. These shadow every layer above, so `# @for-each [1,2] as json` makes `json` the loop item for that request and hides the `json` namespace.
5. The `@assert` shorthands, inside assertions only. `status`, `statusCode`, `statusText`, `header(name)`, and `text()` always refer to the response under test, so a loop variable named `status` remains the loop item everywhere except inside an `@assert`.

Reserved words are never bindable at any layer. The parser rejects them in `@for-each` and `@use`, and the runtime rejects them again.

Shadowing applies to expressions. A module or an `@rts pre-request` block still cannot declare a name that is already bound, so `let json = 1` has always been an error. A local extends that rule to its own name: a request with `# @for-each [1,2] as item` cannot also declare `let item` in its pre-request block. Rename the declaration or the loop variable.

### env

`env` provides environment values. You can access values through `env.get("key")`, `env.has("key")`, `env.require("key"[, msg])`, or `env.key`. `require` throws when a value is missing.

`env.meta.name` is the selected environment, and `env.meta.groups` is the profile in each group. Metadata is separate from the environment value namespace, so an environment value may be called `name`, `groups`, or `meta` without ambiguity.

For grouped environments, `env.meta.name` is the complete selection label, such as `api=dev, app=dev app 1`. Group names use the same matching rules as environment names, and the same `get`, `has`, and `require` helpers are available:

```rts
env.meta.groups.api
env.meta.groups.get("credentials")
env.meta.groups.has("app")
env.meta.groups.require("api", "API profile is required")
```

For named environments, `env.meta.name` is the environment name and `env.meta.groups` is empty.

```rts
env.meta.name     // dev, the selected environment
env.get("name")  // the environment variable called name, if declared
env["name"]      // the same variable
env.NAME          // the same variable, since env names are case-insensitive
```

### vars

`vars` provides request runtime variables, including globals and workflow overrides. You can access values through `vars.get("key")`, `vars.has("key")`, `vars.require("key"[, msg])`, or `vars.key`. `vars.global` provides global reads and writes in pre-request scripts through `get`, `has`, `require`, `set`, and `delete`.

`vars` names the values a run can override, so `@const` values and OS environment variables are not in it even though `{{name}}` resolves them. Everything else follows the same precedence a template does, and a name resolves to one source through both. See [Variable resolution order](resterm.md#variable-resolution-order).

### request

`request` provides a summary of the current request. It exposes `method`, `url`, `headers`, `header(name)`, and `query`. `headers` and `query` are `dict<string, string | list<string>>`: one value is a string, multiple values are a list, and zero values are an empty list. Header keys are lowercased, while query keys are exact. `header(name)` is the explicit first-value convenience lookup. A document header whose name is not an HTTP field name is rejected before evaluation. In `@rts pre-request` blocks, mutation helpers are available, including `request.setMethod`, `request.setURL`, `request.setHeader`, `request.addHeader`, `request.removeHeader`, `request.setQueryParam`, and `request.setBody`. Their string arguments are strict: convert numbers or booleans with `str(...)` yourself. The full `@script pre-request lang=rts` form is equivalent. In `@apply`, the request object is read only, so you return a patch dict instead of mutating it.

### last

`last` provides a summary of the most recent response. It exposes `status`, `statusCode`, `statusText`, `url`, `headers`, `header(name)`, `text()`, and `json(path)`. Header values use the same cardinality-based representation as `request.headers`; `header(name)` returns the first value. `json(path)` accepts a simple dot and `[index]` path (optional leading `$`) and returns null when a value is missing.

For gRPC responses, `headers` merges the response metadata with the trailers, each trailer prefixed with `Grpc-Trailer-`. Values under keys ending in `-bin` are binary and read back base64-encoded without padding, the way they travel on the wire, so a trailer sent as `x-trace-bin` reads as `last.header("grpc-trailer-x-trace-bin")`.

### response

`response` has the same shape as `last`. It is available after the current request completes in `@assert` and both forms of `@capture`, including a `{{= ... }}` expression inside one.

Other expressions run before the current request completes. Use `last` for the previous response in URLs, headers, bodies, and conditions such as `@when`, `@skip-if`, `@for-each`, `@if`, and `@switch`. Using `response` in those contexts is an error that `try` cannot catch.

### trace

`trace` provides timing and budget information for the most recent response. It includes helpers such as `trace.enabled()`, `trace.durationMs()`, `trace.durationSeconds()`, `trace.durationString()`, `trace.error()`, `trace.started()`, `trace.completed()`, `trace.phases()`, `trace.phaseNames()`, `trace.getPhase("dns")`, `trace.budgets()`, `trace.breaches()`, `trace.withinBudget()`, and `trace.hasBudgets()`.

### stream

`stream` provides streaming metadata for SSE and WebSocket requests. It includes helpers such as `stream.enabled()`, `stream.kind()`, `stream.summary()`, and `stream.events()`. Summary and event shapes depend on the stream type (for SSE: `eventCount`, `byteCount`, `duration`, `reason`; for WebSocket: `sentCount`, `receivedCount`, `duration`, `closedBy`, `closeCode`, `closeReason`).

### mock

When the TUI's workspace mock server is running, `mock` provides read-only access to its bounded request journal:

```rts
mock.count({method: "POST", path: "/webhooks/{name}"})
mock.received({
  method: "POST",
  path: "/webhooks/payment",
  query: {page: {gte: 2}, channel: {oneOf: ["web", "ios"]}},
  headers: {
    Authorization: {prefix: "Bearer "},
    "X-Version": {regex: "^v[0-9]+$"},
    "X-Env": {oneOf: ["dev", "prod"]}
  },
  json: {status: "completed"},
  jsonRules: {amount: {gt: 100}, user: {age: {gte: 18}}}
})
```

Both helpers require one pattern dictionary. `count` returns the exact number of matching requests, while `received` returns whether the count is greater than zero. Pattern fields are optional:

- `method` is case-insensitive and normalized to uppercase.
- `path` uses mock path syntax, including `{name}` and terminal `{name...}` wildcards.
- `query` maps case-sensitive names to exact strings/lists or one rule. It accepts every header rule below plus `{gt: 10}`, `{gte: 10}`, `{lt: 10}`, and `{lte: 10}`, which read each value as a number and treat anything else as a non-match.
- `headers` maps case-insensitive names to exact strings/lists or one rule: `{exact: ...}`, `{prefix: "..."}`, `{contains: "..."}`, `{regex: "..."}`, `{oneOf: [...]}`, `{present: true}`, or `{absent: true}`. Header values are case-sensitive, `regex` uses unanchored RE2 syntax, and `oneOf` needs a non-empty array.
- `json` matches a literal body subset. Objects use recursive subset matching, while arrays are exact and ordered. Every key is a field name, including `$schema`, `$ref`, and `gt`.
- `jsonRules` follows the body structure and supports `{gt: ...}`, `{gte: ...}`, `{lt: ...}`, `{lte: ...}`, and `{oneOf: [...]}`. Numeric comparisons require numbers on both sides. Several operators can apply to one value, and `oneOf` compares entire objects and arrays. When `json` and `jsonRules` are both present, both must match.

Journal eviction makes inspection fail instead of returning a potentially false result. Resterm does not connect `resterm run` to an external journal. Use declarative `@expect` entries with `resterm mock verify` for standalone or CI automation.

## Directives and workflows

### @use

```
# @use ./rts/helpers.rts
# @use ./rts/helpers.rts as helpers
```

`@use` is valid at file or request scope. If you omit `as`, the module name declared with `module <name>` becomes the alias.

### @apply

```
# @apply {headers: {"Authorization": "Bearer " + vars.get("auth.token")}}
# @apply use=jsonApi,use=authProd
```

`@apply` is a request scoped directive and you can use it multiple times in a request. Each apply expression is evaluated in order before pre-request scripts. The expression must return a dict patch with specific keys.

Header names in a patch follow the same rule as `request.setHeader`: they must be HTTP field names, whitespace is not trimmed, and a patch naming one header twice is an error rather than a choice made by map order. See [Keys and names](#keys-and-names).

You can also reference reusable named patches with `use=`. Comma-separated `use=` entries run left-to-right inside the same `@apply` line.

### @patch

```
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

### @when and @skip-if

```
# @when vars.has("auth.token")
# @skip-if env.mode == "dry-run"
```

These directives are evaluated before pre-request scripts. If the condition is false, the request is skipped and a reason is reported.

### @assert

```
# @assert response.statusCode == 200
# @assert contains(response.header("Content-Type"), "json")
```

Each expression is evaluated and truthy means pass. Use `response` for the current request response.

### @if, @elif, and @else

These directives are used in workflows to branch steps.

```
# @if last.statusCode == 200 run=StepOK
# @elif last.statusCode == 401 run=StepRefresh
# @else fail="unexpected status"
```

### @switch, @case, and @default

```
# @switch last.statusCode
# @case 200 run=StepOK
# @case 401 run=StepRefresh
# @default fail="unexpected status"
```

These directives route workflow steps and are not the `switch` statement. They share the same equality relation, but each `@case` names a step to run instead of holding a statement list.

### @for-each

```
# @for-each json.file("_data/users.json") as user
```

The expression must evaluate to a list. It introduces a loop variable that you can use in RestermScript expressions. In workflows, it also sets `vars.workflow.<name>` and `vars.request.<name>` for legacy templates.

The loop variable is a local, so it shadows any standard library, host object, or `@use` alias of the same name for the whole request, including `@rts pre-request` blocks. JavaScript pre-request blocks do not see it as a typed value and continue to read `vars.request.<name>`. See [Name precedence](#name-precedence).

## Limits and safety

RestermScript enforces hard limits to prevent runaway scripts and keep the UI responsive. These limits include maximum steps per evaluation, maximum call depth, maximum string size, maximum list size, maximum dict size, and an optional timeout. When a limit is exceeded, evaluation fails with a detailed error.

## Common patterns

### Guarded requests

```
# @when vars.has("auth.token")
GET {{base_url}}/bearer
Authorization: {{= "Bearer " + vars.get("auth.token") }}
```

### Controlled branching in workflows

```
# @switch last.statusCode
# @case 401 run=Refresh
# @case 200 run=Upsert
# @default fail="unexpected status"
```

### Reusable module logic

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

## Design constraints and why they exist

RestermScript prioritizes predictable evaluation and safe execution. It does not allow file writes or network access, and file reads are limited to `json.file` when enabled. It does not allow member assignment because it reduces side effects and simplifies the interpreter. It requires an explicit alias or module name to avoid name collisions and keep imports explicit. It keeps host objects read-only in most contexts because request evaluation should remain declarative. It sorts dict keys during `range` to keep iteration order deterministic across runs.

If you need full scripting or side effects, use JavaScript `@script` blocks. For everything else, RestermScript is the safer and more readable choice.
