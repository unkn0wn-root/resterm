# Quick start

1. Put one or more `.http` or `.rest` files in a directory, or use the samples in `_examples/`.
2. Run `resterm --workspace path/to/project`.
3. In the navigator sidebar, expand a file (`→` or `Space`), highlight a request, and press `Ctrl+Enter` to send it (`Enter` runs, `Space` previews).
4. Check the response in the Pretty, Raw, Headers, Diff, Compare, or History tabs on the right. Press `g+c` to run the current request against the `--compare` targets (or the ones in its `@compare` directive) and see the results without leaving the editor.

A minimal `.http` file looks like this:

```http
### Fetch Status
# @name health
GET https://httpbin.org/status/204
User-Agent: resterm
Accept: application/json

### Create Resource
# @name create
POST https://httpbin.org/anything
Content-Type: application/json

{
  "id": "{{$uuid}}",
  "note": "created from Resterm"
}
```

## Initializing a Project

`resterm init` writes a starter workspace so you can send requests right away:

```bash
mkdir my-api && cd my-api
resterm init
```

Run `resterm` in the same directory. Press `g Shift+M` to start the local mock server, then use `Ctrl+Enter` to try the sample requests. Press `Ctrl+E` to switch between the local `dev` and `test` environments. [`resterm init`](cli/init.md) covers the templates and flags.
