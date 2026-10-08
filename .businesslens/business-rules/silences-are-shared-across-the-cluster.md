---
appliesTo:
  - { type: capability, id: replicate-silences }
  - { type: entity, id: silence }
references:
  - kind: doc
    role: intent
    target: docs/high_availability.md
  - kind: code
    role: implementation
    target: silence/silence.go
---

# Every peer in a cluster keeps the same silences

A silence created, changed or expired on one Alertmanager reaches every peer,
and where two copies of a silence differ the one updated last wins, so
operators can work with any instance of the cluster.
