# Observability & Health Checks

Claudine provides health endpoints and structured log export with W3C Trace Context propagation.

## Health Endpoints

Health endpoints are provided for container orchestration and service management.

*   **Liveness:** `GET /health/liveness`
*   **Readiness:** `GET /health/readiness`

## Log Export

By default, Claudine logs to stdout. You can additionally export logs using OpenTelemetry.

### Exporting Correlated Logs via OTLP

Configure the proxy using standard OpenTelemetry environment variables.

```bash
# Set a service name for easy identification
export OTEL_SERVICE_NAME="claudine"

# Enable the OTLP exporter for logs
export OTEL_LOGS_EXPORTER="otlp"

# Choose transport protocol: http/protobuf (default) or grpc
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"

# Point to your OpenTelemetry collector endpoint (port 4318 for http, 4317 for grpc)
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"

# Run the proxy
claudine start
```

For quick debugging, you can also export directly to the console by setting `OTEL_LOGS_EXPORTER="console"`.
