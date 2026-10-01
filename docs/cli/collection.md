# resterm collection

The collection commands package a workspace so it can be copied or shared safely.

| Command | What it does |
| --- | --- |
| `resterm collection export --workspace <dir> --out <dir>` | Export a Git-friendly bundle directory. |
| `resterm collection import --in <dir> --workspace <dir>` | Import a bundle into another workspace. |
| `resterm collection pack --in <dir> --out <file.zip>` | Pack a bundle directory into a zip archive. |
| `resterm collection unpack --in <file.zip> --out <dir>` | Unpack and validate a bundle archive. |

See [Collection sharing](../collections.md) for the bundle layout, environment files, and safety checks.
