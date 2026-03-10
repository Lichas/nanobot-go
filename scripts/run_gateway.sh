#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GATEWAY_PORT="${GATEWAY_PORT:-18890}"

# Prefer the standalone gateway binary when available.
BIN="$ROOT_DIR/maxclaw-gateway"
if [ ! -x "$BIN" ]; then
  BIN="$ROOT_DIR/build/maxclaw-gateway"
fi

if [ ! -x "$BIN" ]; then
  BIN="$ROOT_DIR/maxclaw"
fi

if [ ! -x "$BIN" ]; then
  BIN="$ROOT_DIR/build/maxclaw"
fi

if [ ! -x "$BIN" ]; then
  echo "Error: gateway binary not found in $ROOT_DIR/{maxclaw-gateway,maxclaw} or build/ equivalents" >&2
  exit 1
fi

case "$(basename "$BIN")" in
  maxclaw-gateway|maxclaw-gateway.exe)
    exec "$BIN" maxclaw-gateway -p "$GATEWAY_PORT"
    ;;
  *)
    exec "$BIN" gateway -p "$GATEWAY_PORT"
    ;;
esac
