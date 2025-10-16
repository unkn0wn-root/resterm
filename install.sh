#!/bin/sh
set -e

REPO="unkn0wn-root/resterm"
INSTALL_DIR="/usr/local/bin"
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

get_latest_release() {
    if command_exists curl; then
        curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | \
            grep '"tag_name":' | \
            sed -E 's/.*"tag_name": "([^"]+)".*/\1/'
    elif command_exists wget; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | \
            grep '"tag_name":' | \
            sed -E 's/.*"tag_name": "([^"]+)".*/\1/'
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

download_binary() {
    local url="$1"
    local output="$2"

    info "Downloading from: $url"

    if command_exists curl; then
        curl -fL -o "$output" "$url" || error "Download failed"
    elif command_exists wget; then
        wget -qO "$output" "$url" || error "Download failed"
    fi
}

main() {
    info "Starting Resterm installation..."

    OS=$(detect_os)
    ARCH=$(detect_arch)
    info "Detected OS: $OS, Architecture: $ARCH"

    info "Fetching latest release..."
    VERSION=$(get_latest_release)
    if [ -z "$VERSION" ]; then
        error "Failed to fetch latest release version"
    fi
    info "Latest version: $VERSION"

    BINARY_FILENAME="resterm_${OS}_${ARCH}"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_FILENAME}"
    TMP_DIR="$(mktemp -d)"
    trap 'rm -rf "$TMP_DIR"' EXIT

    TMP_BINARY="$TMP_DIR/$BINARY_NAME"

    download_binary "$DOWNLOAD_URL" "$TMP_BINARY"
    chmod +x "$TMP_BINARY"

    info "Installing to $INSTALL_DIR..."
    if [ -w "$INSTALL_DIR" ]; then
        mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    else
        info "Requesting sudo privileges to install to $INSTALL_DIR..."
        sudo mv "$TMP_BINARY" "$INSTALL_DIR/$BINARY_NAME"
    fi

    if command_exists "$BINARY_NAME"; then
        INSTALLED_VERSION=$("$BINARY_NAME" --version 2>&1 || echo "unknown")
        info "Successfully installed Resterm!"
        info "Version: $INSTALLED_VERSION"
        info "Location: $(command -v $BINARY_NAME)"
        echo ""
        info "Run 'resterm --help' to get started"
    else
        warn "Installation completed but binary not found in PATH"
        warn "You may need to add $INSTALL_DIR to your PATH"
    fi
}

main
