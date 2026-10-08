---
kind: alternative
routes:
  api: API
steps:
  - text: The Alert generator resends a held Alert with an end time that has passed
    kind: actor
    actor: alert-generator
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      api:
        place: api
  - text: The Product resolves the Alert at that end time
    kind: product
    actor: alert-generator
    entities:
      - { entity: alert, effect: changes, from: Firing, to: Resolved, facts: [Ends at, Updated at] }
    contexts:
      api:
        place: api
---

# Resolve an alert

## Trigger

The alerting rule in the alert generator stops firing.

## Outcome

The alert is resolved and no longer listed; its alert groups send resolved
notifications where their receivers ask for them.
