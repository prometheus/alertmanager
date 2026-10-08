---
appliesTo:
  - type: entity
    id: alert
    effect: removes
permits:
  - unattended: true
references:
  - kind: code
    role: implementation
    target: provider/mem/mem.go
---

# Only Alertmanager itself removes alerts

No one removes an alert; Alertmanager drops resolved alerts at its alert
clean-up interval.
