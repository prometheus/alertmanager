---
kind: alternative
routes:
  web: Web UI
  api: API
steps:
  - text: The end time of an active Silence passes
    kind: condition
    unattended: true
    entities:
      - { entity: silence, effect: reads, facts: [Ends at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
  - text: The Product expires the Silence and stops muting with it, so alerts nothing else mutes are listed as active again
    kind: product
    entities:
      - { entity: silence, effect: changes, from: Active, to: Expired, facts: [] }
      - { entity: alert, effect: changes, facts: [Status, Silenced by] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
---

# Expire a silence at its end time

## Trigger

An active silence's end time passes.

## Outcome

The silence is listed as expired and the alerts it muted are notified again unless something else mutes them.
