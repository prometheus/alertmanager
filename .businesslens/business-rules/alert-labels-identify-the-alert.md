---
appliesTo:
  - type: entity
    id: alert
    facts: [Labels, Fingerprint]
  - { type: capability, id: send-alerts }
references:
  - kind: code
    role: implementation
    target: alert/alert.go
  - kind: code
    role: implementation
    target: provider/mem/mem.go
---

# An alert's labels identify it and never change

Alerts sent with the same labels are the same alert, whatever their
annotations: a resend refreshes the held alert instead of adding another.
Labels with empty values are dropped first, and an alert with different labels
is a different alert.
