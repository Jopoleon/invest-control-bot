#!/usr/bin/env bash
set -euo pipefail

SSH_HOST="${SSH_HOST:-investcontrol-server}"
LOCAL_PORT="${LOCAL_PORT:-6543}"
REMOTE_DB_HOST="${REMOTE_DB_HOST:-127.0.0.1}"
REMOTE_DB_PORT="${REMOTE_DB_PORT:-5432}"
SSH_OPTS=(
  -o ExitOnForwardFailure=yes
  -o ServerAliveInterval=30
  -o ServerAliveCountMax=3
)

printf 'Opening SSH tunnel: 127.0.0.1:%s -> %s:%s via %s\n' \
  "$LOCAL_PORT" "$REMOTE_DB_HOST" "$REMOTE_DB_PORT" "$SSH_HOST"
exec ssh "${SSH_OPTS[@]}" -N -L "${LOCAL_PORT}:${REMOTE_DB_HOST}:${REMOTE_DB_PORT}" "$SSH_HOST"
