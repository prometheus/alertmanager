---
appliesTo:
  - { type: capability, id: send-alerts }
  - { type: capability, id: resolve-alerts }
  - type: entity
    id: alert
    facts: [Ends at]
references:
  - kind: doc
    role: intent
    target: docs/alerts_api.md
  - kind: code
    role: implementation
    target: api/v2/api.go#postAlertsHandler
---

# An alert resolves at its end time unless it is refreshed

An alert sent without an end time ends at its receipt time plus the resolve
timeout of the global settings, five minutes by default; each refresh moves
the end time on. An alert nobody refreshes resolves when its end time passes.
