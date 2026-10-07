# Workspaces and files

- Resterm scans the workspace root for `.http` and `.rest` files. Use `--workspace` to set the root. Without it, Resterm uses the directory of the file passed with `--file`. Add `--recursive` to scan subdirectories too. Hidden directories are skipped.
- The navigator also lists environment files, marked with an `ENV` badge, and the files your requests use, such as `.rts` modules and `.graphql`, `.json` or `.js` files. Select one to open it in the editor.
- The navigator filter sits above the tree. Press `/` to focus it, then type to match files, request and workflow names, URLs, tags, and badges. `m` toggles the method badge of the highlighted request (one at a time), `t` toggles tag badges, and `Esc` clears the text and any badges.
- In Git workspaces, supported files show short status markers in the navigator (`M`, `A`, `U`, `D`, `R`, `!`), and the status bar shows the current branch and how many supported files changed. Unsupported files are ignored even when they have Git changes.
- Highlight a request or workflow in the navigator and press `l` or `r` to restore the editor pane and jump to that definition.
- The navigator refreshes as soon as a file is saved or reparsed. Filtering also loads files you haven't opened yet, so matches from the whole workspace still show up. Use `Ctrl+Shift+O` (or `g+Shift+O`) to rescan the workspace for new files.
- Press `g+e` to open the current file, or the file of the selected navigator item, in an external editor. Resterm uses `$RESTERM_EDITOR`, then `$VISUAL`, then `$EDITOR`. If none of them is set or found, it shows a warning. It does not fall back to a default app that isn't an editor.
- Resterm watches the active file on disk. If another tool edits or deletes it, a modal tells you the file changed or went missing. Your buffer in Resterm stays as it is. Press the reload shortcut (`g+Shift+R` by default, or whatever you've mapped to `reload_file_from_disk`) to load the version on disk into the editor. If you have unsaved changes, the first press warns that reloading will discard them. Press reload again to confirm. Press `Esc` to keep your buffer and continue editing.
- Press `Ctrl+T` to open a scratch buffer for quick tests. Scratch buffers are not written to disk unless you save them.

## Inline requests

You can send simple requests without a `.http` file:

1. Type `GET https://api.example.com/users` (or just the URL) in the editor.
2. Place the cursor on the line and press `Ctrl+Enter`.

Inline requests support full URLs and a limited curl import:

```bash
curl \
  -X POST https://api.example.com/login \
  -H "Content-Type: application/json" \
  -d '{"user":"demo","password":"pass"}'
```

Resterm recognizes common curl flags (`-X`, `--request`, `-H`, `--header`, `-d/--data*`, `--json`, `--url`, `-u/--user`, `--head`, `--compressed`, `-F/--form`) and turns them into a request. Commands split over several lines with backslashes work too. For larger curl scripts or multiple commands, use the CLI importer (`--from-curl`).
