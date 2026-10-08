---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: silence/silence.go#canUpdate
  - kind: code
    role: implementation
    target: api/v2/api.go#postSilencesHandler
  - kind: code
    role: implementation
    target: cli/silence_update.go
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceForm/Types.elm
---

# Edit silence

An operator changes an existing silence. A change that keeps the matchers and,
for an active silence, its start and an end not yet passed, or, for a pending
silence, a start still in the future, keeps the silence and its ID. Any other
change keeps a new silence under a new ID and expires the original, so the
original still tells what was muted when.
