
![Build Status](https://github.com/SPS-SIPS/p-monitoring/actions/workflows/build.yml/badge.svg)

## Features

- **Flexible health checks:** Monitors any number of HTTP endpoints, each representing a component or service.
- **Smart status detection:** Accepts JSON (`{"status":"ok"}`), plain text `ok`, or any HTTP 200 response as healthy.
- **Live status API:** Exposes `/health` endpoint showing the latest status of all components in a single JSON response.
- **Configurable:** All settings (components, check interval, log directory, retention, listen address) are set in `config.json`.
- **Structured logging:** Logs every check in JSON format, with log rotation and retention.
- **Resource efficient:** No unnecessary dependencies; suitable for lightweight and containerized environments.
- **Tested:** Includes unit tests and benchmarks for core health check and aggregation logic.

## Configuration

Create a `config.json` file in the working directory. Example:

```json
{
  "components": [
    {
      "name": "zitadel",
      "endpoint": "http://localhost:8080/debug/healthz"
    },
    {
      "name": "minio",
      "endpoint": "http://localhost:9000/minio/health/live"
    }
  ],
  "check_interval_seconds": 30,
  "log_directory": "logs",
  "log_retention_days": 7,
  "listen_address": ":8081"
}
```

### Configuration Options

- `components`: List of components to monitor. Each must have:
  - `name`: A unique name for the component (string).
  - `endpoint`: The HTTP health endpoint to call (string).
- `check_interval_seconds`: How often to check all endpoints (integer, seconds).
- `log_directory`: Directory for log files (string).
- `log_retention_days`: How many days to keep log files (integer).
- `listen_address`: Address and port for the HTTP server (string, e.g., `":8081"`).

### Adding New Components

To add a new component, add an entry to the `components` array in `config.json`:

```json
{
  "name": "myservice",
  "endpoint": "http://localhost:1234/health"
}
```

The application will call this endpoint every interval and include its status in the `/health` response.

## Building

Ensure you have Go 1.20 or newer installed.

```
go build -o participant-monitor
```

## Running

```
./participant-monitor -config config.json
```

- `-config`: Path to the configuration file (default: `config.json`)

## API

- `GET /health` — Returns the current status of all registered components as JSON.

Example response:

```json
{
  "status": "ok",
  "components": [
    {
      "name": "zitadel",
      "status": "ok",
      "endpoint_status": "ok",
      "http_result": "200 OK",
      "last_checked": "2025-07-28T12:34:56Z"
    },
    {
      "name": "minio",
      "status": "ok",
      "endpoint_status": "ok",
      "http_result": "200 OK",
      "last_checked": "2025-07-28T12:34:56Z"
    }
  ]
}
```

- If any component is not "ok", the top-level status will be "degraded".

## Logging

- Logs are written in JSON format to the directory specified by `log_directory` in the config.
- Log files are rotated daily and old logs are deleted according to `log_retention_days`.

## Testing

Run all unit tests:

```
go test -v
```

## Benchmarking

Run benchmarks for the status logic:

```
go test -bench=.
```

## Project Structure

- `main.go` — Main application logic
- `main_test.go` — Unit tests and benchmarks
- `config.json` — Sample configuration file

## License

MIT License
