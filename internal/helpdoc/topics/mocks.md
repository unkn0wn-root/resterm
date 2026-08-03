# Mock Servers

Define mock routes in request files, then control the workspace server with `:mock`.

```http
# @mock method=GET path=/users/{id} default=true
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"{{path.id}}"}
```

Select responses conditionally with `@match`, chain them with `sequence=`, and assert call counts with `@expect`. Useful commands are `:mock start`, `:mock logs`, `:mock reset`, and `:mock verify`.

`:mock start` serves the whole workspace. Narrow it to named files with `:mock start --source users.http,payments.http`, widen it again with `:mock restart --all`, and check the active scope with `:mock status`. The scope is remembered like the address, so a restart or a later start keeps serving the same files.
