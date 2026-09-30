# Workspaces and files

- Resterm scans the workspace root for `.http` and `.rest` files. Use `--workspace` to set the root or rely on the directory of the file passed via `--file`. Add `--recursive` to traverse subdirectories (hidden directories are skipped).
- The navigator filter sits above the tree: press `/` to focus, type to match files, request/workflow names, URLs, tags, and badges. `m` toggles method badges (single select) for the highlighted request, `t` toggles tag badges, and `Esc` clears text plus any badges.
- In Git workspaces, supported files show compact status markers in the navigator (`M`, `A`, `U`, `D`, `R`, `!`), and the status bar shows the current branch plus supported-file change counts. Unsupported files are ignored even when they have Git changes.
- Highlight a request or workflow in the navigator and press `l` or `r` to restore the editor pane and jump to that definition.
- The navigator refreshes immediately when a file is saved or reparsed; filtering auto-loads unopened files so cross-workspace matches still appear. Use `Ctrl+Shift+O` (or `g+Shift+O`) to rescan the workspace for new files.
- Press `g+e` to open the current file, or the selected navigator file/request/workflow file, in an external editor. Resterm uses `$RESTERM_EDITOR`, then `$VISUAL`, then `$EDITOR`; if none is configured or found, it shows a warning instead of falling back to a non-editor default app.
- Resterm watches the active file on disk. If another tool edits or deletes it, a modal appears telling you the file changed or went missing. Your in-memory buffer stays intact. Press the reload shortcut (`g+Shift+R` by default, or whatever you’ve mapped to `reload_file_from_disk`) to pull the on disk version into the editor. If you have unsaved changes, the first press warns that reload will discard them; press reload again to confirm. Dismiss with `Esc` to keep your buffer and continue editing.
- Create a scratch buffer with `Ctrl+T` for ad-hoc experiments. These buffers are not written to disk unless you save them explicitly.

## Inline requests

You can execute simple requests without a `.http` file:

1. Type `GET https://api.example.com/users` (or just the URL) in the editor.
2. Place the cursor on the line and press `Ctrl+Enter`.

Inline requests support full URLs and a limited curl import:

```bash
curl \
  -X POST https://api.example.com/login \
  -H "Content-Type: application/json" \
  -d '{"user":"demo","password":"pass"}'
```

Resterm recognizes common curl flags (`-X`, `--request`, `-H`, `--header`, `-d/--data*`, `--json`, `--url`, `-u/--user`, `--head`, `--compressed`, `-F/--form`) and converts them into a structured request. Multiline commands joined with backslashes are supported. For larger curl scripts or multiple commands, use the CLI importer (`--from-curl`).
