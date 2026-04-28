# Codex MCP: Prod PostgreSQL via SSH Tunnel

This repository keeps Codex MCP config local to the project.
The production PostgreSQL is expected to be reachable only through an SSH tunnel.

## 1. Start the tunnel

From the repository root:

```bash
bash scripts/prod_postgres_tunnel.sh
```

Defaults:
- SSH host: `investcontrol-server`
- local forwarded port: `6543`
- remote PostgreSQL host: `127.0.0.1`
- remote PostgreSQL port: `5432`

Override them if needed:

```bash
SSH_HOST=investcontrol-server LOCAL_PORT=6543 REMOTE_DB_HOST=127.0.0.1 REMOTE_DB_PORT=5432 bash scripts/prod_postgres_tunnel.sh
```

Keep this process running while Codex uses the MCP.

## 2. Add repo-local MCP entry

Add this block to `.codex/config.toml` in this repository:

```toml
[mcp_servers.investcontrol_prod_postgres]
command = "/home/egor/.nvm/versions/node/v20.20.0/bin/node"
args = [
  "/home/egor/.local/mcp/postgres/node_modules/@modelcontextprotocol/server-postgres/dist/index.js",
  "postgresql://investcontrol_app:REPLACE_DB_PASSWORD@127.0.0.1:6543/investcontrol_prod?sslmode=disable"
]
startup_timeout_sec = 30.0

[mcp_servers.investcontrol_prod_postgres.env]
npm_config_cache = "/tmp/.npm"
```

Notes:
- replace `REPLACE_DB_PASSWORD` with the real production DB password
- if the password contains special characters, URL-encode them
- keep the existing `wsl_local_postgres` entry; this prod entry is additive

## 3. Start a clean Codex session

Start a new session from the repository root:

```bash
cd ~/Work/src/github.com/Jopoleon/invest-control-bot
codex
```

Do not rely on `codex resume` from another repository if you expect repo-local MCP config to be applied.

## 4. Expected behavior

With the tunnel running and `.codex/config.toml` updated, a fresh Codex session should see:
- `wsl_local_postgres`
- `investcontrol_prod_postgres`

## Operational cautions

- This MCP gives database read/query access from the local Codex session into production over SSH tunnel.
- Only start it intentionally.
- Shut down the tunnel when finished.
- Prefer running destructive SQL manually outside of MCP.
