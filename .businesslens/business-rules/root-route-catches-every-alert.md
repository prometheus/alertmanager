---
appliesTo:
  - { type: capability, id: route-alerts }
  - { type: entity, id: route }
references:
  - kind: code
    role: implementation
    target: config/config.go
---

# Every alert reaches at least one receiver

The root route matches every alert and names a receiver, so an alert that no
deeper route matches is routed by the root route.
