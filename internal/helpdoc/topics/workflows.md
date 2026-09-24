# Workflows

Workflows run named requests as ordered steps and can branch or loop with RestermScript expressions.

```http
# @workflow provision
# @step Login using=login
# @step Create using=createUser expect.statuscode=201
```

`vars.workflow.*` values persist between steps and `vars.request.*` values apply to one step. Select a workflow in the navigator and press `Enter` to run it.

Use `@run var` in a workflow to share one value across its steps. Put it on a request to reuse the value when that request runs again in the same workflow. The next run gets a new value.

```http
# @workflow provision
# @run var suffix = {{$fake.word}}
# @step Create using=createUser
```

If a value fails to evaluate, Resterm does not send the affected request. It marks the step failed and follows its `on-failure` setting.

Workflow names are required, and `on-failure` accepts `stop` or `continue`. An unknown directive between `@workflow` and the next request is a parse error, so a mistake such as `@stpe` cannot silently remove a step. Directives attached to requests remain request-scoped, even when the workflow runs those requests. To write directive-looking prose, add another comment marker, for example `## @step ...`.

Related: `:help rts`, `:help variables`.
