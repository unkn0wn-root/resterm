# Workflows

Workflows run named requests as ordered steps and can branch or loop with RestermScript expressions.

```http
# @workflow provision
# @step Login using=login
# @step Create using=createUser expect.statuscode=201
```

`vars.workflow.*` values persist between steps and `vars.request.*` values apply to one step. Select a workflow in the navigator and press `Enter` to run it.

Related: `:help rts`, `:help variables`.
