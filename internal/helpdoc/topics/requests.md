# Requests

A request starts with a method and URL. Metadata comments immediately above it describe how Resterm should run or display it.

```http
### Users
# @name listUsers
# @tag users smoke
GET {{apiUrl}}/users
Accept: application/json
```

Start each request with a `###` separator. Comments may use `#`, `//`, or `--`. The body follows a blank line after the headers, or comes from a file with `< ./payload.json`.

Related: `:help variables`, `:help authentication`, `:help transport`.
