# Participant-monitor security changes — 16 November 2026

- `GET /health` is now a minimal anonymous probe: only `{"status":"ok"}` or `{"status":"degraded"}`. It does not disclose component names, endpoint results or error messages.
- Detailed health moves to `GET /diagnostics`. Configure `STATUS_API_TOKEN` with at least 32 characters and send `Authorization: Bearer <token>`. Missing token configuration disables diagnostics with 503; missing/wrong credentials return 401. Comparisons use constant-time SHA-256 digest comparison.
- The listener defaults to `127.0.0.1:8080`. The tracked example now binds `127.0.0.1:8081`. Configure `listen_address` explicitly if another address is required.
- Non-loopback listeners now require `TLS_CERT_FILE` and `TLS_KEY_FILE` and use HTTPS. Supplying only one of the two also fails startup. Literal loopback addresses can use HTTP. An existing `:8081`/`0.0.0.0:8081` configuration without TLS will stop with a clear error instead of exposing diagnostics over plain HTTP.
- The server uses its own HTTP mux, bounded timeouts and `no-store`/`nosniff` response headers. Health/diagnostics accept GET only.
- CI now runs race tests and pinned `govulncheck`; Dependabot checks Go modules and GitHub Actions weekly.

Clients that read `components` from `/health` must switch to authenticated `/diagnostics`, and remote clients must use the configured TLS listener. Simple status-only probes can retain `/health`. These are separate credentials from outbound webhook credentials used by other Go services.

The existing `participant-monitor.service` file-mode change was present before this task and has been preserved. No service, listener, TLS certificate or token was deployed or changed on a server by this task.
