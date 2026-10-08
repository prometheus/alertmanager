---
kind: alternative
routes:
  api: API
steps:
  - text: The Alert generator resends a firing alert whose labels match an Alert Alertmanager holds
    kind: actor
    actor: alert-generator
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Starts at, Ends at] }
    contexts:
      api:
        place: api
  - text: The Product merges it into the held Alert, keeping the earliest start time and taking the newer annotations, generator URL and end time
    kind: product
    actor: alert-generator
    entities:
      - { entity: alert, effect: changes, facts: [Annotations, Ends at, Updated at, Generator URL] }
    contexts:
      api:
        place: api
---

# Refresh a firing alert

## Trigger

The alert generator resends an alert that is still firing.

## Outcome

The held alert stays firing with a later end time, and no second alert is
created.

## Edge cases

- When the resent alert's time range does not overlap the held one, it replaces the held alert instead of being merged.
- An end time that came from the resolve timeout gives way to a newer end time even when the newer one is earlier.
