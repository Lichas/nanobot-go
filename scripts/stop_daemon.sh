#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PID_DIR="$ROOT_DIR/.pids"

stop_pid() {
  local name="$1"
  local pid_file="$PID_DIR/$name.pid"
  if [ -f "$pid_file" ]; then
    local pid
    pid="$(cat "$pid_file")"
    if kill -0 "$pid" >/dev/null 2>&1; then
      echo "Stopping $name (PID $pid)"
      kill "$pid" >/dev/null 2>&1 || true
    else
      echo "$name not running (stale PID $pid)"
    fi
    rm -f "$pid_file"
  else
    echo "$name PID file not found"
  fi
}

stop_pid bridge
stop_pid gateway
