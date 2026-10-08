# resterm collection

Use the collection commands to bundle a workspace's request files and inputs, then import them into another workspace.

| Command | What it does |
| --- | --- |
| `resterm collection export --workspace <dir> --out <dir>` | Export a Git-friendly bundle directory. |
| `resterm collection import --in <dir> --workspace <dir>` | Import a bundle into another workspace. |
| `resterm collection pack --in <dir> --out <file.zip>` | Pack a bundle directory into a zip archive. |
| `resterm collection unpack --in <file.zip> --out <dir>` | Unpack and validate a bundle archive. |

`export`, `pack`, and `unpack` stop when the output path already exists. Pass `--force` to replace it. `import` takes `--force` to replace existing files and `--dry-run` to preview the changes.

These commands exit `0` on success, `2` for a missing or unknown flag, extra arguments, or an unknown subcommand, and `1` for any other error.

See [Collection sharing](../collections.md) for the bundle layout, environment files, and safety checks.
