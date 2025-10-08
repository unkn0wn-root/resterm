#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$ROOT_DIR/bin"

mkdir -p "$OUT_DIR"

platforms=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
  "windows arm64"
)

for platform in "${platforms[@]}"; do
  read -r goos goarch <<<"$platform"

  case "$goos" in
    darwin) goos_label="Darwin" ;;
    linux) goos_label="Linux" ;;
    windows) goos_label="Windows" ;;
    *) goos_label="$goos" ;;
  esac

  case "$goarch" in
    amd64) goarch_label="x86_64" ;;
    arm64) goarch_label="arm64" ;;
    *) goarch_label="$goarch" ;;
  esac

  output="$OUT_DIR/resterm_${goos_label}_${goarch_label}"
  if [[ "$goos" == "windows" ]]; then
    output+=".exe"
  fi

  echo "Building $output"
  GOOS="$goos" GOARCH="$goarch" go build -o "$output" ./cmd/resterm
done
