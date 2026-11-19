#!/usr/bin/env bash
# Builds the desktop app: desktop/build/bin/secrio-desktop.app (macOS).
# Extra arguments go to `wails build` (e.g. -debug for devtools).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$PATH:$(go env GOPATH)/bin"

# The desktopapp embed needs frontend/dist to exist before Wails compiles it.
mkdir -p "$root/desktop/frontend/dist"
[ -n "$(ls -A "$root/desktop/frontend/dist")" ] || touch "$root/desktop/frontend/dist/.keep"

(cd "$root/desktop/frontend" && pnpm install --frozen-lockfile)

cd "$root/cmd/secrio-desktop"
# -m: skip `go mod tidy`; -nosyncgomod: keep go.mod's Wails version as is.
wails build -tags desktopapp -m -nosyncgomod "$@"
# wails build regenerates the committed bindings with mode 0755; keep 0644.
find "$root/desktop/frontend/wailsjs" -type f -exec chmod 644 {} +
