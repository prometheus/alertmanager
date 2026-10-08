---
appliesTo:
  - { type: capability, id: create-silence }
  - { type: capability, id: edit-silence }
  - { type: capability, id: import-silences }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#postSilencesHandler
---

# A submitted silence ends after it starts and not in the past

A silence whose end is not after its start, or whose end has already passed,
is refused. A start in the past becomes the moment the silence is kept.
