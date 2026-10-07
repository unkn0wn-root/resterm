# resterm init

`resterm init` creates a starter set of files in a directory so you can start sending requests right away.

```bash
# Create files in the current directory
resterm init

# Create files in a subdirectory
resterm init ./api-tests

# Use the minimal template
resterm init --template minimal

# Preview what would be created
resterm init --dry-run

# Overwrite existing files
resterm init --force
```

The generated sample does not need an external service. Start it with `resterm mock requests.http`. Open another terminal and run a request such as `resterm run --request CreateUser requests.http`.

## Templates

| Template | Files created |
| --- | --- |
| `standard` (default) | `requests.http`, `resterm.env.json`, `resterm.env.example.json`, `rts/helpers.rts`, `RESTERM.md` |
| `minimal` | `requests.http`, `resterm.env.json` |

Both templates add `resterm.env.json` to `.gitignore` so secrets stay out of version control. If you prefer to manage `.gitignore` yourself, pass `--no-gitignore`.

## Flags

| Flag | Description |
| --- | --- |
| `--dir <path>` | Target directory. You can also pass the path as a positional argument, but not both. |
| `--template <name>` | Template to use (`standard` or `minimal`). |
| `--force` | Overwrite existing files instead of aborting. If any file cannot be written, no files are changed. |
| `--dry-run` | Print actions without writing anything. |
| `--no-gitignore` | Skip updating `.gitignore`. |
| `--list` | Print available templates and exit. |

`resterm init` exits `0` on success, `2` for an unknown flag, extra arguments, or an unknown template, and `1` for any other error, including a file that already exists without `--force`.
