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

1. Download the archive for your platform from the [GitHub Releases](https://github.com/unkn0wn-root/resterm/releases) page. There are builds for macOS, Linux, and Windows, on amd64 and arm64.
2. Make the binary executable (`chmod +x resterm` on Unix), then copy it into a directory on your `PATH`.
3. Run `resterm --help` to check that it works.

Prebuilt Linux binaries need glibc 2.32 or newer. On an older distro, build from source or upgrade glibc.

The commands below do the same download from a terminal. The Unix version needs `curl` and `jq`.

```bash
# Find the latest release tag
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unkn0wn-root/resterm/releases/latest | jq -r .tag_name)

# Download the matching binary (Darwin/Linux + amd64/arm64)
curl -fL -o resterm "https://github.com/unkn0wn-root/resterm/releases/download/${LATEST_TAG}/resterm_$(uname -s)_$(uname -m)"

# Install on PATH
chmod +x resterm
sudo install -m 0755 resterm /usr/local/bin/resterm
```

```powershell
$latest = Invoke-RestMethod https://api.github.com/repos/unkn0wn-root/resterm/releases/latest
$asset  = $latest.assets | Where-Object { $_.name -like 'resterm_Windows_*' } | Select-Object -First 1
Invoke-WebRequest -Uri $asset.browser_download_url -OutFile resterm.exe
# Optionally move to a directory on PATH:
Move-Item resterm.exe "$env:USERPROFILE\bin\resterm.exe"
```

## Build from source

```bash
go install github.com/unkn0wn-root/resterm/cmd/resterm@latest
```

This needs Go 1.25 or newer. The binary goes into `$(go env GOPATH)/bin`.

## Updating

If you installed with Homebrew, update with `brew upgrade resterm`. If you used a release binary or the install scripts, run `resterm --check-update` to see if there is a new version, and `resterm --update` to download, verify, and install it in place. On Windows, the old binary stays next to the new one as `resterm.exe.old` and is removed on the next update.
