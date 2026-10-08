---
appliesTo:
  - { type: capability, id: send-alerts }
  - { type: entity, id: alert }
references:
  - kind: doc
    role: intent
    target: docs/alertmanager.md
  - kind: code
    role: implementation
    target: limit/bucket.go
  - kind: code
    role: implementation
    target: store/store.go
---

# New alerts beyond the per-alert-name limit are dropped

When the operator sets a per-alert-name limit, Alertmanager holds at most that
many unresolved alerts with the same alert name. Refreshes of alerts it
already holds are still accepted, and resolved alerts make room for new ones.
The limit is off unless set.

## Rationale

An unexpectedly high number of instances of one alert would otherwise threaten
Alertmanager's reliability and flood the receivers.
