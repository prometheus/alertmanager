---
appliesTo:
  - { type: capability, id: start-alertmanager }
  - { type: capability, id: stop-alertmanager }
  - { type: entity, id: silence }
  - { type: entity, id: notification }
references:
  - kind: code
    role: implementation
    target: silence/silence.go#Silences.Maintenance
  - kind: code
    role: implementation
    target: nflog/nflog.go
---

# Silences and the notification log survive a restart

Alertmanager writes silences and the notification log to its storage path at
every maintenance interval and when it stops, and restores them when it
starts. Alerts are not written.
