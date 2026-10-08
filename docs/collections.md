# Collection sharing

Export a collection bundle to share request files and their inputs with another developer, a CI job, or a support environment. You can commit the bundle to Git and import it into another workspace.

A bundle contains your request files and the files they use: RTS modules, script includes, payloads, GraphQL queries and variables, gRPC descriptors and messages, and WebSocket `send-file` payloads. Its `manifest.json` records a checksum for each file. Import checks those checksums before writing anything to disk.

## What happens to environment files

The bundle includes an example environment file. Your working environment file is handled as follows:

1. If `resterm.env.example.json` exists in the workspace, Resterm exports it exactly as written.
2. If only `resterm.env.json` or `rest-client.env.json` exists, Resterm generates `resterm.env.example.json` and replaces every value with `REPLACE_ME`. In grouped files, the group and profile keys and the `$default` strings stay as they are. Only the values under `$shared` and the profiles are redacted.
3. If no environment file exists, Resterm still writes an empty `resterm.env.example.json`, so every bundle has the same layout.

## Export a collection bundle

This command exports the workspace recursively and gives the bundle a name:

```bash
resterm collection export \
  --workspace ./my-api \
  --out ./shared/my-api-bundle \
  --recursive \
  --name "my-api-v1"
```

A typical bundle directory looks like this:

```text
shared/my-api-bundle/
  manifest.json
  requests.http
  rts/helpers.rts
  payloads/create-user.json
  resterm.env.example.json
```

You can commit this directory to Git and review it like any other files in your project.

## Import a collection bundle

You can import that bundle into a local workspace with:

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api
```

To see what import will do before it writes anything, run:

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api \
  --dry-run
```

If files already exist in the destination and you want to replace them, add `--force`.

## Pack a bundle into a zip archive

To hand off a single file instead of a directory, pack an existing bundle:

```bash
resterm collection pack \
  --in ./shared/my-api-bundle \
  --out ./shared/my-api-bundle.zip
```

This command checks the manifest and the file checksums before it writes the archive, so you don't pack a damaged bundle by accident.

## Unpack a zip archive back into a bundle directory

You can unpack the archive before importing it:

```bash
resterm collection unpack \
  --in ./shared/my-api-bundle.zip \
  --out ./shared/my-api-bundle
```

Unpack checks archive paths and rejects unsafe entries, such as traversal paths and symlinks. It then checks the checksums against `manifest.json`, and only after that moves the unpacked bundle into place.

## Safety and validation behavior

Export, import, pack, and unpack all check paths and file integrity. Resterm rejects references that point outside the workspace, traversal paths in manifests and archives, and symlinks that escape. An operation fails if a file's size or checksum does not match the manifest.

Export, pack, and unpack refuse to write to an output path that already exists. Add `--force` to replace it. Resterm builds the new bundle or archive first and only then deletes the old output. For export and unpack that is the whole directory, so point `--out` at a directory that holds nothing but the bundle.
