---
appliesTo:
  - { type: capability, id: edit-silence }
  - type: entity
    id: silence
    facts: [Matchers, Starts at]
references:
  - kind: code
    role: implementation
    target: silence/silence.go#canUpdate
---

# A silence keeps its ID only while what it muted stays true

A silence keeps its ID through a change only when its matchers stay the same
and, once it is active, its start stays the same and its end is not moved into
the past. Any other change keeps a new silence under a new ID and expires the
original, and an expired silence is never changed, so a silence ID always
tells which alerts it muted and when.
