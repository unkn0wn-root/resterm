# Collection sharing

Resterm can export a collection bundle that holds everything your requests need. You can commit it to Git and import it anywhere else. This helps when you want to hand the exact same requests and inputs to another developer, a CI job, or a support environment.

A bundle contains your request files and the files they depend on, such as RTS modules, script includes, payload files, GraphQL query/variables files, gRPC descriptor/message files, and WebSocket `send-file` payloads. Resterm writes a `manifest.json` file with a checksum for each file, and import checks those checksums before it writes anything to disk.

## What happens to environment files

Resterm shares environments as a safe example file, not as your real environment file:

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
