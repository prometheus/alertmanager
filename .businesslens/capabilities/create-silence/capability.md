---
domain: silences
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#postSilencesHandler
  - kind: code
    role: implementation
    target: silence/silence.go
  - kind: code
    role: implementation
    target: ui/app/src/Views/SilenceForm/Updates.elm
  - kind: code
    role: implementation
    target: cli/silence_add.go
---

# Create silence

An operator creates a silence from label matchers, a start and an end, naming who
created it and why. In the web UI a new silence can start from an alert's
labels, from the current alert filter or from an expired silence, and the
operator can preview the alerts it would affect.
