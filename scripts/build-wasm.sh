#!/usr/bin/env bash
# scripts/build-wasm.sh — compile internal/physics to WebAssembly.
#
# Produces:
#   web/public/physics.wasm   — the Go-compiled WASM binary
#   web/public/wasm_exec.js   — the Go runtime JS shim (from GOROOT)
#
# Usage:
#   ./scripts/build-wasm.sh
#
# Requirements:
#   - Go toolchain (standard, NOT TinyGo — TinyGo substitutes math/runtime
#     and breaks the Go<->WASM bit-identical parity guarantee)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_WASM="${REPO_ROOT}/web/public/physics.wasm"
OUT_JS="${REPO_ROOT}/web/public/wasm_exec.js"
GOROOT="$(go env GOROOT)"
WASM_EXEC="${GOROOT}/lib/wasm/wasm_exec.js"

mkdir -p "${REPO_ROOT}/web/public"

echo "Building physics.wasm (GOOS=js GOARCH=wasm)…"
GOOS=js GOARCH=wasm go build \
  -trimpath \
  -ldflags='-s -w' \
  -tags 'js wasm' \
  -o "${OUT_WASM}" \
  "${REPO_ROOT}/cmd/physicswasm"

echo "Copying wasm_exec.js from GOROOT…"
cp "${WASM_EXEC}" "${OUT_JS}"

echo ""
echo "Done."
ls -lh "${OUT_WASM}" "${OUT_JS}"
