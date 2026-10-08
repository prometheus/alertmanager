---
kind: alternative
routes:
  web: Web UI
  api: API
steps:
  - text: The start time of a pending Silence arrives
    kind: condition
    unattended: true
    entities:
      - { entity: silence, effect: reads, facts: [Starts at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
  - text: The Product begins muting the alerts the Silence matches
    kind: product
    entities:
      - { entity: silence, effect: changes, from: Pending, to: Active, facts: [] }
      - { entity: alert, effect: changes, facts: [Status, Silenced by] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
---

# Start a pending silence

## Trigger

A scheduled silence's start time comes.

## Outcome

The silence is listed as active and mutes the alerts it matches.
