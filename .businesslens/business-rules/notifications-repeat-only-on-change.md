---
appliesTo:
  - { type: capability, id: send-notifications }
  - { type: entity, id: alert-group }
references:
  - kind: code
    role: implementation
    target: notify/dedup_stage.go
---

# A group is notified again only when it changed or its repeat interval passed

For each integration, an alert group is notified again only when new alerts
fire in it, when alerts resolve and the integration sends resolved
notifications, or when the repeat interval has passed since the last
notification. A group that was never notified sends nothing about alerts that
resolved.
