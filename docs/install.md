# Install

## Homebrew and install scripts

macOS and Linux:

```bash
brew install resterm
# or
curl -fsSL https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.sh | bash
```

Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/unkn0wn-root/resterm/main/install.ps1 | iex
```

## Prebuilt binaries

1. Download the archive for your platform from the [GitHub Releases](https://github.com/unkn0wn-root/resterm/releases) page (macOS, Linux, or Windows; amd64 and arm64 builds are published).
2. Mark the binary as executable (`chmod +x resterm` on Unix), then copy it into a directory on your `PATH`.
3. Launch with `resterm --help` to confirm the CLI is available.

Prebuilt Linux binaries need glibc 2.32 or newer. On an older distro, build from source or upgrade glibc.

## Build from source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

This requires Go 1.25 or newer. The binary will be installed in `$(go env GOPATH)/bin`.

## Updating

Homebrew installs update with `brew upgrade resterm`. Binaries from the releases page or the install scripts use `resterm --check-update` and `resterm --update`, which downloads, verifies, and installs in place. On Windows the old binary stays next to the new one as `resterm.exe.old` and is removed on the next update.
