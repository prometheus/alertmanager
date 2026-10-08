---
availability:
  - { place: api }
references:
  - kind: code
    role: implementation
    target: httpserver/httpserver.go#Register
  - kind: code
    role: implementation
    target: notify/notify.go
  - kind: doc
    role: context
    target: docs/alertmanager.md
---

# Read metrics

Operators scrape an Alertmanager's metrics in the Prometheus format: alerts
received and held, notifications sent and failed, silences, configuration
reloads, clustering and API requests.
