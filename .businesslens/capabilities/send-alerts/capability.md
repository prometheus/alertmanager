---
domain: alerts
availability:
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#postAlertsHandler
  - kind: code
    role: implementation
    target: provider/mem/mem.go
  - kind: code
    role: implementation
    target: alert/alert.go
  - kind: code
    role: implementation
    target: cli/alert_add.go
  - kind: doc
    role: intent
    target: docs/alerts_api.md
---

# Send alerts

Alert generators send the alerts they fire and resolve, and keep resending
firing alerts so Alertmanager knows they still fire. An alert whose labels
match one Alertmanager already holds refreshes that alert instead of adding a
new one. An operator can add an alert by hand with amtool, typically to test
routing and receivers.
