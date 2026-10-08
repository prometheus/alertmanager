---
appliesTo:
  - { type: capability, id: route-alerts }
  - { type: entity, id: route }
references:
  - kind: code
    role: implementation
    target: dispatch/route.go
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# A route inherits receiver, grouping, timing and labels but not time intervals

A route that does not set its receiver, grouping labels, group wait, group
interval or repeat interval takes its parent's, and its route labels add to
and override its parent's. Mute and active time intervals apply only to the
route that names them.
