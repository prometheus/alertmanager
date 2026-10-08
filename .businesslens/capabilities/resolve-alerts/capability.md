---
domain: alerts
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: alert/alert.go
  - kind: code
    role: implementation
    target: api/v2/api.go#alertFilter
  - kind: doc
    role: intent
    target: docs/alerts_api.md
---

# Resolve alerts

An alert whose end time passes without a refresh from its alert generator
resolves on its own: it is no longer listed and its alert groups treat it as
resolved.
