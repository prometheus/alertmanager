---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: The end time of a firing Alert passes without a refresh
    kind: condition
    unattended: true
    entities:
      - { entity: alert, effect: reads, facts: [Ends at] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product treats the Alert as resolved from its end time and stops listing it
    kind: product
    entities:
      - { entity: alert, effect: changes, from: Firing, to: Resolved, facts: [] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Resolve an alert nobody refreshes

## Trigger

An alert generator stops resending an alert, for example because it stopped
running.

## Outcome

The alert resolves at its end time; for an alert sent without an end time
that is the resolve timeout, five minutes by default, after its last refresh.
It stops inhibiting other alerts and its groups may send resolved notifications.
