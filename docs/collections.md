# Collection sharing

Resterm can export a portable, self-contained collection bundle that you can commit to Git and import anywhere else. This is useful when you want a reliable "same inputs, same requests" handoff between developers, CI jobs, or support environments.

A bundle export contains your request files plus the files those requests depend on, such as RTS modules, script includes, payload files, GraphQL query/variables files, gRPC descriptor/message files, and WebSocket `send-file` payloads. Resterm writes a `manifest.json` file with per-file checksums, and import verifies those checksums before writing to disk.

## What happens to environment files

Resterm treats environment sharing as an explicit safe template flow:

1. If `resterm.env.example.json` exists in the workspace, Resterm exports it exactly as written.
2. If only `resterm.env.json` or `rest-client.env.json` exists, Resterm generates `resterm.env.example.json` and replaces every value with `REPLACE_ME`. In grouped files the group and profile keys and the `$default` strings are kept as they are, only the values under `$shared` and the profiles are redacted.
3. If no environment file exists, Resterm still writes an empty `resterm.env.example.json` so the bundle shape remains predictable.

## Export a collection bundle

The following command exports recursively and names the bundle:

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

You can commit this directory directly to Git and open it in code review like any other project files.

## Import a collection bundle

You can import that bundle into a local workspace with:

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api
```

If you want to inspect the plan before writing, run:

```bash
resterm collection import \
  --in ./shared/my-api-bundle \
  --workspace ./my-local-api \
  --dry-run
```

If destination files already exist and replacement is intentional, you can add `--force`.

## Pack a bundle into a zip archive

If you want to hand off a single file instead of a directory, you can pack an existing bundle:

```bash
resterm collection pack \
  --in ./shared/my-api-bundle \
  --out ./shared/my-api-bundle.zip
```

This command reads and validates the bundle manifest and payload checksums before writing the archive, so you do not package a partially corrupted bundle by accident.

## Unpack a zip archive back into a bundle directory

You can unpack the archive before importing it:

```bash
resterm collection unpack \
  --in ./shared/my-api-bundle.zip \
  --out ./shared/my-api-bundle
```

Unpack validates archive paths, rejects unsafe entries (for example traversal paths and symlinks), validates checksums against `manifest.json`, and only then moves the unpacked bundle into place.

## Safety and validation behavior

Export, import, pack, and unpack all enforce path safety and integrity checks. Resterm rejects references that escape the workspace, rejects malicious traversal paths in manifests and archives, rejects symlink escapes, and fails operations if size or checksum validation does not match the manifest.
