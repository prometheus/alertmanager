---
appliesTo:
  - type: entity
    id: silence
    effect: removes
permits:
  - unattended: true
references:
  - kind: code
    role: implementation
    target: silence/silence.go#Silences.GC
---

# Only Alertmanager itself removes silences

No one deletes a silence; operators expire it, and Alertmanager removes it
once it has been expired for the data retention period.
