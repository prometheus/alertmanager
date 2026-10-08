---
references:
  - kind: code
    role: implementation
    target: cluster/cluster.go
  - kind: doc
    role: intent
    target: docs/high_availability.md
---

# Peer

Another Alertmanager in the same cluster, with which this one shares silences
and the notification log.

## Information kept

- **Name** — the peer's name in the cluster
- **Address** — the address the peer gossips on
