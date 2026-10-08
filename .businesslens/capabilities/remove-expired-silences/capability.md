---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: silence/silence.go#Silences.GC
  - kind: code
    role: implementation
    target: silence/silence.go#Silences.Maintenance
---

# Remove expired silences

At every maintenance run Alertmanager removes the silences that expired longer
ago than the data retention period, then writes its silences to disk.
