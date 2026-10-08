---
kind: alternative
routes:
  web: Web UI
  api: API
steps:
  - text: A new Alert arrives whose route and grouping label values match an existing Alert group
    kind: condition
    unattended: true
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
      - { entity: alert-group, effect: reads, facts: [Group labels] }
      - { entity: route, effect: reads, facts: [Group by] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product adds the Alert to that Alert group without bringing the group's next send forward
    kind: product
    entities:
      - { entity: alert-group, effect: changes, facts: [] }
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Add an alert to an existing group

## Trigger

An alert arrives that belongs in an alert group already notified or waiting.

## Outcome

The alert is notified with the rest of its group at the group's next notification.
