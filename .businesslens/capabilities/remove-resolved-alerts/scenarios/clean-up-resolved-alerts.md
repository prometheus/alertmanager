---
kind: primary
routes:
  api: API
steps:
  - text: The clean-up interval elapses
    kind: condition
    unattended: true
    entities: []
    contexts:
      api:
        place: api
  - text: The Product removes every resolved Alert it holds
    kind: product
    entities:
      - { entity: alert, effect: removes, from: Resolved }
    contexts:
      api:
        place: api
---

# Clean up resolved alerts

## Trigger

The alert clean-up interval, thirty minutes by default, elapses.

## Outcome

Resolved alerts are no longer held; an alert generator that later sends the
same labels again starts a new alert. No alert history is kept.
