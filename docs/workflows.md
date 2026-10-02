# Workflows

Group existing requests into repeatable workflows using `@workflow` blocks. Each step references a request by name and can override variables or expectations.

```http
### Provision account
# @workflow provision-account on-failure=continue
# @step Authenticate using=AuthLogin expect.statuscode=200
# @step CreateProfile using=CreateUser vars.request.name={{vars.workflow.userName}}
# @step FetchProfile using=GetUser

### AuthLogin
POST https://example.com/auth

### CreateUser
POST https://example.com/users

### GetUser
GET https://example.com/users/{{vars.workflow.userId}}
```

Workflows parsed from the current document appear in the **Workflows** list on the left. Select one and press `Enter` (or `Space`) to run it. Resterm executes each step in order, respects `on-failure=continue`, and streams progress in the status bar. When the run completes, the **Workflow** tab shows a summary, a stable step list, and the response for the selected step. Resterm selects the first failed or canceled step by default, or the first step when everything passes. Press `Enter` or `Space` on a selected step to focus its response detail and scroll long responses without changing the selected step. One combined entry is written to history so you can review the results later.

Key directives and tokens:

- `@workflow <name>` starts a workflow. The name is required and cannot be replaced by an option. Add `on-failure=<stop|continue>` to change the default behavior. Other tokens, such as `region=us-east-1`, are kept under `Workflow.Options` for tooling. Empty or invalid `on-failure` values are parse errors on both `@workflow` and `@step`.
- `@description` / `@tag` lines inside the workflow build the description and tag list shown in the UI and stored in history.
- `@step <optional-alias>` defines an execution step. Supply `using=<RequestName>` (required), `on-failure=<...>` for per-step overrides, `expect.status` / `expect.statuscode`, and any number of `vars.*` assignments. The alias is the first word, so quote it when it holds spaces or an equals sign (`@step "Create Account" using=CreateUser`). `name=` sets it instead when the step starts with an option.
- `vars.request.*` keys add step-scoped values that are available as `{{vars.request.<name>}}` during that request. They do not rewrite existing `@var` declarations automatically, so reference the namespaced token (or copy it in a pre-request script) when you want the override.
- `vars.workflow.*` keys persist between steps and are available anywhere in the workflow as `{{vars.workflow.<name>}}`, so later requests can reuse or change shared values such as `vars.workflow.userId`.
- `@run var <name> = <value>` gives the steps one shared value. See [Run variables](#run-variables).
- Unknown tokens on `@workflow` or `@step` are kept in `Options`, so custom scripts or future features can use them without changing the file format.
- An unknown directive between `@workflow` and the next request is a parse error. Directives attached to requests remain request-scoped, even when the workflow runs those requests. Resterm continues parsing valid workflow steps to report other problems, but it will not run the file until the error is fixed.
- `expect.status` supports quoted or escaped values, so you can write `expect.status="201 Created"` alongside `expect.statuscode=201`.
- `expect.status` / `expect.statuscode` require non-empty values, and `expect.statuscode` must be numeric.

> **Tip:** Workflow assignments are expanded when a request runs. Use `@run var` when every step needs the same value from a helper such as `{{$uuid}}`.

> **Tip:** Options are parsed like CLI flags. Wrap values in quotes or escape spaces (`\ `) to keep text together, for example `expect.status="201 Created"`.

Every workflow run is saved in History next to regular requests. The newest entry is highlighted, so you can open the generated `@workflow` definition and results from the History pane right after the run.

## Run variables

Use `@run var` in a workflow block to share one value across its steps. Put it on a request to reuse a value each time that request runs within the workflow.

```http
# @workflow create-order
# @run var suffix = {{$fake.word}}-{{$randomInt(1000, 9999)}}
# @step Create using=CreateOrder
# @step Fetch using=GetOrder

### CreateOrder
# @name CreateOrder
POST https://example.com/orders
Content-Type: application/json

{"reference": "order-{{suffix}}"}

### GetOrder
# @name GetOrder
GET https://example.com/orders/order-{{suffix}}
```

- Workflow values are set before the first step. A request's values are set the first time that request runs. Later steps and loop iterations reuse them. A request value takes priority over a workflow value with the same name.
- Each new run gets new values. A request sent on its own gets new values each time, while a request with `@for-each` shares its values across iterations. `@profile` and `@compare` set new values for each execution.
- Declarations are set from top to bottom. A value can use helpers, other variables, and the `@run var` declarations above it, but not the ones below it. A request value can build on the workflow value it replaces, for example `# @run var suffix = {{suffix}}-retry`.
- Templates, conditions, loops, and scripts can read the values with `{{suffix}}` or `vars.get("suffix")`. Once set, their contents are plain text and are not expanded again.
- If a declaration fails, Resterm does not send the affected request. The error points to the declaration, and `on-failure` decides whether later steps run. A failed workflow declaration affects every step that runs.
- Names ignore case and cannot be repeated in the same workflow or request. Run values are public, so `env:` references are rejected. Put the `env:` reference in `@file` or `@request`, then use that variable.
