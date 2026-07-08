#!/bin/bash
set -e

REPO="unkn0wn-root/resterm"
INSTALL_DIR="$HOME/.local/bin"
BINARY_NAME="resterm"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() {
    printf "${GREEN}[INFO]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

detect_os() {
    case "$(uname -s)" in
        Linux*)     echo "Linux";;
        Darwin*)    echo "Darwin";;
        MINGW*|MSYS*|CYGWIN*) echo "Windows";;
        *)          error "Unsupported operating system: $(uname -s)";;
    esac
}

detect_arch() {
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64)   echo "x86_64";;
        aarch64|arm64)  echo "arm64";;
        *)              error "Unsupported architecture: $ARCH";;
    esac
}

fetch() {
    if command_exists curl; then
        curl -fsSL -H "Accept: application/vnd.github+json" "$1"
    elif command_exists wget; then
        wget -qO- --header="Accept: application/vnd.github+json" "$1"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# the release api publishes a sha256 digest for every asset; "name" comes
# before "digest" inside each asset object, so remember the last name seen
digest_for() {
    printf '%s' "$RELEASE_JSON" | awk -F'"' -v name="$1" '
        $2 == "name" { found = ($4 == name) }
        found && $2 == "digest" { sub(/^sha256:/, "", $4); print $4; exit }
    '
}

download_binary() {
    local url="$1"
    local output="$2"

    info "Downloading from: $url"

    if command_exists curl; then
        curl -fL --progress-bar -o "$output" "$url" || error "Download failed"
    elif command_exists wget; then
        wget --show-progress -qO "$output" "$url" || error "Download failed"
    fi
}

sha256_of() {
    if command_exists sha256sum; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

verify_checksum() {
    local file="$1" expected="$2" actual
    command_exists sha256sum || command_exists shasum || \
        error "Need sha256sum or shasum to verify the download"
    actual="$(sha256_of "$file")"
    if [ "$actual" != "$expected" ]; then
        error "Checksum mismatch for $file: expected $expected, got $actual"
    fi
    info "Checksum verified"
}

main() {
    info "Starting Resterm installation..."

    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "Detected OS: $OS, Architecture: $ARCH"

    case "$SHELL" in
        */bash) shellRc="$HOME/.bashrc" ;;
        */zsh) shellRc="$HOME/.zshrc" ;;
        */ksh) shellRc="$HOME/.kshrc" ;;
        */sh) shellRc="$HOME/.shrc" ;;
        *)
            shellRc="$HOME/.profile"
            warn "Shell $SHELL not directly supported, using .profile for PATH"
            ;;
    esac

    mkdir -p "$INSTALL_DIR"
    touch "$shellRc"

    grep -F 'export PATH=$PATH:$HOME/.local/bin' "$shellRc" > /dev/null || {
        echo 'export PATH=$PATH:$HOME/.local/bin' >> "$shellRc"
        info "Added \$HOME/.local/bin to PATH in $shellRc"
        info "To use resterm immediately, run: . $shellRc or restart your session"
    }

    info "Fetching latest release..."
    RELEASE_JSON="$(fetch "https://api.github.com/repos/${REPO}/releases/latest")"
    VERSION=$(printf '%s' "$RELEASE_JSON" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    if [ -z "$VERSION" ]; then
        error "Failed to fetch latest release version"
    fi
    info "Latest version: $VERSION"

    BINARY_FILENAME="resterm_${OS}_${ARCH}"
    EXPECTED_SHA256="$(digest_for "$BINARY_FILENAME")"
    printf '%s' "$EXPECTED_SHA256" | grep -qE '^[0-9a-f]{64}$' || \
        error "No sha256 digest for $BINARY_FILENAME in release $VERSION"

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILENAME}"
    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    TMP_BINARY="$TMP_DIR/$BINARY_NAME"

    download_binary "$DOWNLOAD_URL" "$TMP_BINARY"
    verify_checksum "$TMP_BINARY" "$EXPECTED_SHA256"
    chmod +x "$TMP_BINARY"

    info "Installing to $INSTALL_DIR..."
    mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"

    info "✔ Successfully installed Resterm $VERSION!"
    info "Location: $INSTALL_DIR/$BINARY_NAME"
}

main
