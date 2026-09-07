# Quick Start

Create a `.http` or `.rest` file, write a request, and press `Enter` in the navigator or `Ctrl+Enter` in the editor.

```http
# @name health
GET https://example.com/health
```

- `Tab` moves focus between panes.
- `Space` previews the selected request without sending it.
- `i` enters editor insert mode and `Esc` returns to normal mode.
- Warnings and errors refresh when you leave insert mode. In normal mode, press `K` for details and `] d` / `[ d` to move between diagnostics.

Run `:help requests` for request-file syntax and `:help interface` for the layout.
