---
availability:
  - { place: server }
references:
  - kind: code
    role: implementation
    target: eventrecorder/recorder.go
  - kind: code
    role: implementation
    target: eventrecorder/config.go
  - kind: spec
    role: intent
    target: proto/eventrecorder/events/v2/events.proto
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# Record events

While the `event-recorder` feature is enabled, Alertmanager records
significant events — start and shutdown, alerts received, grouped and
resolved, silences created, updated or muting alerts, inhibitions, and
notifications sent — to the files, standard output, webhooks and Kafka topics
its configuration names.
