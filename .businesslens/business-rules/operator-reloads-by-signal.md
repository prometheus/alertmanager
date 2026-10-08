---
appliesTo:
  - type: entity
    id: configuration
    effect: changes
    contexts:
      - { place: server }
permits:
  - actors: [operator]
references:
  - kind: code
    role: implementation
    target: app/lifecycle.go
  - kind: doc
    role: intent
    target: docs/management_api.md
---

# Only the operator running Alertmanager reloads it by signal

A hang-up signal reaches Alertmanager only from someone allowed to signal its
process on the host it runs on; Alertmanager itself makes no further check.
