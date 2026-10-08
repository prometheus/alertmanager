---
type: api
actors: [alert-generator, operator]
entryPoints:
  - api: /api/v2/
  - api: /-/reload
  - api: /-/healthy
  - api: /-/ready
  - api: /metrics
references:
  - kind: spec
    role: intent
    target: api/v2/openapi.yaml
    title: Alertmanager API v2 OpenAPI specification
  - kind: doc
    role: intent
    target: docs/management_api.md
    title: Management API
  - kind: code
    role: implementation
    target: api/v2/api.go
  - kind: code
    role: implementation
    target: api/api.go
  - kind: code
    role: implementation
    target: httpserver/httpserver.go
---

# Alertmanager API

The HTTP API alert generators send alerts to and that operators, scripts, the
web UI and amtool use to read alerts, alert groups, receivers and status and
to manage silences, together with the management endpoints for health,
readiness, metrics and configuration reload.
