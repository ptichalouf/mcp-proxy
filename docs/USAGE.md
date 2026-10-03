# Usage

## CLI

```text
-config string         path to config file or a http(s) url (default "config.json")
-expand-env            expand environment variables in config file (default true)
-http-headers string   optional headers for config URL: 'Key1:Value1;Key2:Value2'
-http-timeout int      timeout (seconds) for remote config fetch (default 10)
-insecure              skip TLS verification for remote config
-authorize string      run a one-time interactive OAuth authorization for the
                        named mcpServers entry, then exit
-auth-status           list every configured server with its transport and
                        authentication state, then exit
-doctor                like -auth-status, but also connects to each server to
                        confirm its credentials are accepted right now
-web                   serve the management dashboard and its API, which can
                        add, edit, and remove servers in the config file
-check-config          load and validate the config, then exit
-log-level value       log level: debug, info, warn, or error (default info)
-version               print version and exit
-help                  print help and exit
```

## Validating configuration

Use `-check-config` in CI, init containers, or deployment scripts to validate
the proxy settings and every downstream server without binding the HTTP port:

```bash
mcp-proxy -config config.json -check-config
# Config OK: 3 MCP server(s) configured
```

Validation includes transport requirements, absolute HTTP URLs, OAuth callback
safety, authentication tokens, and tool-filter modes. Invalid configuration
exits non-zero with the affected field or server name.

## Endpoints

Given `mcpProxy.baseURL = https://mcp.example.com` and a server key `fetch`:

- For `type: sse`: `https://mcp.example.com/fetch/sse`
- For `type: streamable-http`: `https://mcp.example.com/fetch/mcp`

## Health checks

Two unauthenticated endpoints are always served for liveness/readiness probes
(Docker, reverse proxies, dashboards, monitoring):

- `/_healthz` (liveness) returns `200` as soon as the process serves requests. It
  is about this process only, so it stays `200` even when a downstream is down.
- `/_readyz` (readiness) returns `503` with `"status":"initializing"` until every
  enabled server has finished connecting and mounting its route, then `200`.
- `/_readyz` returns `503` again with `"status":"degraded"` if a downstream that
  had connected later stops answering: each connection is probed every 30s, and
  the servers that failed are listed in `unhealthy`. It takes three failed probes
  in a row, so a busy single-threaded server that misses one probe does not take
  the proxy out of rotation. Servers speaking the modern protocol (2026-07-28,
  which removed ping) are probed with a real `tools/list` request; legacy
  connections are still pinged.
- `/_readyz` returns `503` with `"status":"unavailable"` if no enabled server is
  mounted at all. Nothing can be named as broken in that case, but no MCP route
  can serve.
- `GET` returns a JSON status document; `HEAD` returns the same code with an
  empty body. `serverCount` counts enabled servers only.

```bash
curl http://127.0.0.1:9090/_healthz
# {"name":"MCP Proxy","serverCount":3,"status":"ok","version":"1.0.0"}

curl http://127.0.0.1:9090/_readyz
# {"name":"MCP Proxy","serverCount":3,"status":"degraded","unhealthy":["notion"],"version":"1.0.0"}
```

### Routes of servers that are not connected

A server's route (`/<name>/mcp`, `/<name>/sse`, `/<name>/message`) is published
only once its backend connects. Until then, and whenever it is stopped, the
route answers JSON instead of falling through to the dashboard (which used to
give a `405` to `POST` and the dashboard HTML to `GET`):

| Situation | Status | Body |
| --- | --- | --- |
| Configured, not connected (down, retrying, failed) | `502` | `{"error":"upstream <name> unreachable"}` |
| Configured but `disabled` | `503` | `{"error":"server <name> disabled"}` |
| Not configured | `404` | `{"error":"unknown server <name>"}` |

The server's `authTokens` are checked first, exactly as on the connected route,
so a caller without a valid token gets `401`, not the state of the backend. The
route serves again as soon as the backend reconnects, without a restart. Each
`502` logs a `WARN` `upstream unreachable` with `server=<name>`, at most once per
server and minute. Every other path keeps its previous answer (the dashboard
with `-web`, a plain `404` without).

A server that never connected at startup is *not* reported as unhealthy: it has
no route, and keeping the whole proxy out of rotation would take the working
servers down with it. Use `-doctor` or the startup logs to find those.

By default a server that is down at startup stays down until the proxy is
restarted, and a connection that drops stays `degraded`. Set `autoReconnect: true`
on a server (or in `mcpProxy.options` for all of them) to have it repair itself:
the proxy retries an unreachable backend every `reconnectInterval` (default `15s`)
and mounts its route once it connects, and it rebuilds a connection that drops.
This trades the external-restart contract for in-process recovery, which suits a
host where the proxy outlives the app it fronts (e.g. started at login).

The two cases use different timings. A backend that was never up is retried on
`reconnectInterval`. A connection that drops after being healthy is noticed by the
keepalive probe instead, so its recovery is governed by `pingInterval` and the
three-consecutive-failure threshold — about 90s with the defaults. Lower
`pingInterval` to detect and rebuild a drop sooner.

These endpoints never require the proxy auth token, which also means the
`unhealthy` list exposes your server names to anyone who can reach the port.
Bind the proxy to an internal address, or keep the health endpoints on an
internal route in your reverse proxy, if those names are sensitive.

## Auth

If `options.authTokens` is set for a server, requests must include the token in
the `Authorization` header. Both forms are accepted, and the scheme name is
case-insensitive:

```
Authorization: Bearer <token>
Authorization: <token>
```

If your client cannot set headers, embed the token in the route key (e.g. `fetch/<token>`) and call that path instead.

## OAuth-authorizing a downstream server

For servers configured with an `oauth` block (see [CONFIGURATION.md](CONFIGURATION.md#oauth)),
run the authorization flow once, by hand, before starting (or restarting)
the daemon:

```bash
mcp-proxy -authorize notion -config path/to/config.json
```

This opens your default browser to the provider's consent screen, waits
for the local redirect callback, exchanges the code for a token, and saves
it to disk. Run it interactively, in a session with a real browser -
never from an unattended service/container, since it requires you to log
in and approve access.

Once authorized, (re)start the daemon. A server's HTTP route is only
mounted on a successful connect at startup, so if the daemon was already
running when you authorized, restart it now - the new token won't be
picked up otherwise. After that, tokens refresh automatically as they
expire with no further restarts needed. Re-run `-authorize` only if the
server reports the token is no longer valid (e.g. access was revoked).

## Management dashboard

`-web` serves a dashboard at `/` for adding, editing, enabling, renaming and
removing downstream servers without restarting the proxy:

```bash
mcp-proxy -config config.json -web
```

It is **off by default**, and worth being deliberate about: an endpoint that can
edit the config file can start processes as the user the proxy runs as. When
`mcpProxy.options.authTokens` is set, the dashboard's API is gated by those same
tokens. The page itself is served without one — it has to be, so it can ask —
and it then prompts for a token, verifies it against a real endpoint, and stores
it in this browser's `localStorage`. Until a valid token is supplied the
dashboard shows nothing but the prompt, and a sign-out button in the header
forgets it. Without `authTokens`, `-web` starts **only** if `mcpProxy.addr`
is explicitly bound to loopback (`127.0.0.1`, `::1`, or `localhost`). An
unauthenticated wildcard/LAN listener is rejected; configure tokens before
binding the dashboard remotely. `-web` also refuses to start when the config
is a remote `http(s)` URL, since there is nothing to write back to.

A change is applied by reloading the config and starting or stopping only the
servers that actually changed, so connections belonging to untouched servers
are not disturbed. Every write is validated before it lands (a config that
would not load is rejected and the file is left as it was), the previous file is
backed up to `<config dir>/.backups/` keeping the 10 most recent, and the write
is atomic.

`${VAR}` placeholders are preserved in the file rather than replaced by
expanded values; configure secrets in the proxy process environment and put
placeholders in the dashboard. **A literal secret entered in the install form
is saved in plaintext in `config.json`**, so do not paste production credentials
there. The dashboard masks configured secret fields in responses, but that is
not encryption at rest and arguments/URLs should never contain credentials.

The `mcp-proxy -web` process serves the compiled UI from the binary itself;
there is no separate frontend to deploy.

### Dashboard API

All of these live under `/api/mcp/` and require the auth token when one is
configured. They answer JSON, and an unknown `/api/` path is a JSON 404 rather
than the dashboard's HTML page.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/mcp/installed` | Configured servers, with secrets masked |
| `POST` | `/api/mcp/install` | Add a server (from scratch or a catalog entry) |
| `PATCH` | `/api/mcp/servers/{id}` | Rename, enable/disable, edit env, args, command or URL |
| `DELETE` | `/api/mcp/servers/{id}` | Remove a server |
| `GET` | `/api/mcp/servers/{id}/logs` | Recent stdout/stderr/system lines for a server |
| `GET` | `/api/mcp/catalog` | The built-in catalog, filtered by `?q=` and `?category=` |
| `GET` | `/api/mcp/proxy/health` | Health, uptime and the config's state |
| `POST` | `/api/mcp/proxy/reload` | Re-read the config from disk and apply the diff |

The catalog is compiled in rather than fetched from a third-party index at
runtime, so the dashboard works offline and adds no supply-chain dependency.
Entries only describe how to launch a server (`npx`, `uvx`, `docker`, …);
installing is exactly what the command does, as if you had written the config
by hand.
