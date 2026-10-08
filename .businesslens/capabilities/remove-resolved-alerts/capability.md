---
domain: alerts
availability:
  - { place: api }
references:
  - kind: code
    role: implementation
    target: provider/mem/mem.go
  - kind: code
    role: implementation
    target: store/store.go
---

# Remove resolved alerts

At every alert clean-up interval Alertmanager stops holding the alerts that
have resolved. Alert groups still notifying about them keep their own copy
until the resolved notification has gone out.
