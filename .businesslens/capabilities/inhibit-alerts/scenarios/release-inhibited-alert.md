---
kind: alternative
routes:
  web: Web UI
  api: API
steps:
  - text: The inhibiting Alert resolves
    kind: condition
    unattended: true
    entities:
      - { entity: alert, as: source, effect: reads, facts: [Ends at] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product stops inhibiting the target Alert, which is notified again with its Alert group unless something else mutes it
    kind: product
    entities:
      - { entity: alert, as: target, effect: changes, facts: [Status, Inhibited by] }
      - { entity: alert-group, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Release an inhibited alert

## Trigger

The wider problem is over while a narrower alert still fires.

## Outcome

The narrower alert is listed as active and notified again from its group's next notification.
