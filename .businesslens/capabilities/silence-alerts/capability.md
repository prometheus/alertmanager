---
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: silence/silence.go#Silencer.Mutes
  - kind: code
    role: implementation
    target: api/v2/api.go#alertFilter
  - kind: doc
    role: intent
    target: docs/alertmanager.md
---

# Silence alerts

Alertmanager mutes every firing alert that all matchers of an active silence
select: the alert is listed as silenced and left out of notifications. A
pending silence starts muting on its own at its start time.
