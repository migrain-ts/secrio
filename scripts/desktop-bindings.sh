#!/usr/bin/env bash
# Regenerates the committed TypeScript bindings (desktop/frontend/wailsjs)
# from desktop.API. Run after changing a bound method or a DTO.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$PATH:$(go env GOPATH)/bin"

# The generator compiles the desktopapp embed, which needs frontend/dist to
# exist (fresh clone: it does not).
mkdir -p "$root/desktop/frontend/dist"
[ -n "$(ls -A "$root/desktop/frontend/dist")" ] || touch "$root/desktop/frontend/dist/.keep"

cd "$root/cmd/secrio-desktop"
# `wails generate module` ignores build:tags in wails.json.
wails generate module -tags desktopapp
