# Mock Servers

Define mock routes in request files, then control the workspace server with `:mock`.

```http
# @mock method=GET path=/users/{id} default=true
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"{{path.id}}"}
```

Select responses conditionally with `@match`, chain them with `sequence=`, and assert call counts with `@expect`. Useful commands are `:mock start`, `:mock logs`, `:mock reset`, and `:mock verify`.
