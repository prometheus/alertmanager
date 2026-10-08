---
appliesTo:
  - { type: capability, id: send-notifications }
  - { type: entity, id: notification }
references:
  - kind: doc
    role: intent
    target: docs/high_availability.md
  - kind: code
    role: implementation
    target: notify/dedup_stage.go
  - kind: code
    role: implementation
    target: notify/cluster_stages.go
---

# Peers in a cluster send each notification once while they can reach each other

Every peer receives the same alerts and evaluates the same alert groups, but
each waits its turn by its position in the cluster and skips a notification a
peer has already recorded in the shared notification log. When peers cannot
reach each other, each side notifies on its own.

## Rationale

Alert generators send every alert to every Alertmanager of a cluster; without
this, each receiver would hear about every group once per peer, and the
cluster prefers duplicate notifications to missing ones.
