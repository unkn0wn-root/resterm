# Statements

## let and const

```rts
let name = expr
const name = expr
```

`let` creates a mutable binding and `const` creates an immutable binding. Redeclaring a name in the same scope is an error, while shadowing a name in an inner block is allowed. Assignment requires the name to exist in the current or parent scope.

## Assignment

```rts
name = expr
```

Assignment only applies to variable names. Member assignment and index assignment are not supported.

## Functions

```rts
fn add(a, b) {
  return a + b
}
```

Functions close over their lexical environment. Function names are immutable because `fn` defines a constant binding. Function parameters are local variables and can be reassigned.

## Conditionals

```rts
if cond {
  ...
} elif other {
  ...
} else {
  ...
}
```

Conditionals evaluate each branch in order and execute the first branch whose condition is true. The `else` branch runs only when no earlier condition is true.

## switch

`switch` picks one branch out of many. The tagged form compares a value against cases, and the tagless form replaces a long `if`/`elif` chain.

```rts
switch response.statusCode {
case 200, 201:
  result = "success"
case 401:
  result = "unauthorized"
default:
  result = "unexpected"
}
```

```rts
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

```text
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

```rts
switch ({a: 1}).a {
case 1:
  matched = true
}
```

Case expressions are arbitrary runtime expressions, so duplicate cases are not reported at parse time. The first one written wins.

`default` is a reserved word, so it is always the clause label and never a name. Use `??` for a fallback value:

```rts
switch value {
default:
  value = candidate ?? "fallback"
}
```

Switch initializers, type switches, switch expressions that produce a value, and `fallthrough` are not part of the language. A switch can carry a label so that a `break` deeper inside can leave it by name, described under [Labels](#labels).

This statement is separate from the `@switch` workflow directive. `@switch` selects a workflow step in an `.http` file and shares the same equality relation, while `switch` is a statement inside RestermScript code.

## for loops

RTS supports several loop forms.

```rts
for { ... }
for cond { ... }
for let k, v range expr { ... }
```

The language also supports a three clause loop with init, condition, and post clauses. The clauses are separated by the semicolon token.

Rules for loops are consistent. `continue` is valid only inside loops, and `break` is valid inside loops and switches. Both accept a label to target an enclosing statement by name. `const` is not allowed in loop headers. `for let` introduces loop scoped variables that do not escape the loop block. `for range` without `let` assigns to existing variables.

## range semantics

Range iteration is deterministic and follows clear rules.

- When you range a list, the key is the index and the value is the item.
- When you range a dict, the key is the string key and the value is the item, and keys are sorted to keep output stable.
- When you range a string, the key is the byte index and the value is a single rune string.

Example:

```rts
for let i, ch range "go" {
  // i is the byte index, ch is "g" and then "o"
}
```

## Labels

A label names a `for` or a `switch` so that a `break` or `continue` deeper inside can target it by name. Without labels there is no way to leave a loop from inside a switch, because a plain `break` stops at the switch.

```rts
outer: for let i = 0; i < 10; i = i + 1 {
  switch i {
  case 5:
    break outer
  }
}
```

`continue label` resumes the named loop, skipping the rest of every construct in between:

```rts
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

```text
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
