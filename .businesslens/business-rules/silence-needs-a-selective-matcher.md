---
appliesTo:
  - { type: capability, id: create-silence }
  - { type: capability, id: edit-silence }
  - { type: capability, id: import-silences }
  - type: entity
    id: silence
    facts: [Matchers]
references:
  - kind: code
    role: implementation
    target: silence/silence.go#validateSilence
---

# A silence has at least one matcher that does not match an empty label

A silence must name at least one matcher, and not all of its matchers may
match a label that is empty or missing, so no silence mutes every alert.
