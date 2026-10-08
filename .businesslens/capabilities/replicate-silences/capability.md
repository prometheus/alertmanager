---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: silence/silence.go
  - kind: code
    role: implementation
    target: cluster/cluster.go
  - kind: doc
    role: intent
    target: docs/high_availability.md
---

# Replicate silences

While clustering is on, every Alertmanager takes in the silences its peers
gossip to it, so a silence created, changed or expired on one instance can be
seen and worked with on any of them.
