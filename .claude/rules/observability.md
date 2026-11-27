---
paths:
  - "*/internal/app/**"
  - "*/cmd/**"
  - "*/pkg/**/*.go"
  - "shared/pkg/logger/**"
  - "shared/pkg/metrics/**"
  - "shared/pkg/tracing/**"
  - "shared/pkg/telemetry/**"
  - "deploy/compose/observability/**"
---

# Observability

- **Setup** — `internal/app` calls `telemetry.Init(ctx, serviceName, logger.Init,
  tracing.InitTracer[, metrics.Init])` before anything else and
  `telemetry.Flush(...)` last in `Close`. Service names are `<svc>-service`
  (`order-service`, …) — e2e and dashboards query by them. Don't read OTel
  env vars yourself: exporters use the standard `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `telemetry.Disabled()` the standard `OTEL_SDK_DISABLED`.
- **Logs** — keep using `log/slog`; use the `*Context` variants whenever a
  ctx exists so records carry `trace_id`/`span_id`. Log business events on
  success paths (`order created`, `parts reserved`, …) with IDs as attributes.
  Never log secrets: passwords, password hashes, **session UUIDs**.
- **Traces** — every gRPC server gets `grpc.StatsHandler(otelgrpc.NewServerHandler())`,
  every gRPC client `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`, HTTP
  servers are wrapped in `otelhttp.NewHandler` (outermost). Span names must be
  low-cardinality: no IDs in names (see `spanRoute` in `order/internal/app`).
  Propagate ctx through every call — a `context.Background()` in a request
  path breaks the trace.
- **Metrics** — business metrics go through a consumer-owned `Metrics`
  interface in the service package (no-op for tests, mock in unit tests),
  implemented with the OTel meter in `<svc>/pkg/metrics`; record only after
  the state change committed and not on idempotent no-ops. Name instruments in
  Prometheus style without the `_total` suffix (the remote-write translation
  adds `_total` and UCUM unit suffixes like `_seconds`); keep non-UCUM units
  (currency) in the name. Labels must be low-cardinality (payment method yes,
  order UUID never). New series → add a panel to
  `deploy/compose/observability/grafana/dashboards/orders.json` and, if it
  matters, an e2e `prom_at_least` check.
- **Backends** — config changes in `deploy/compose/observability/` are
  verified with `task test:e2e`, which asserts logs per service in
  `otel-logs`, business metrics in Prometheus and cross-service traces in Jaeger.
