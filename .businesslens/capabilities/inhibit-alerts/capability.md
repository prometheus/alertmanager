---
domain: alerts
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: inhibit/inhibit.go
  - kind: code
    role: implementation
    target: inhibit/cache.go
  - kind: doc
    role: intent
    target: docs/alertmanager.md
---

# Inhibit alerts

While a firing alert matches the source side of an inhibition rule, alerts
matching its target side and sharing its equal labels are inhibited: they are
left out of notifications and shown as inhibited.
