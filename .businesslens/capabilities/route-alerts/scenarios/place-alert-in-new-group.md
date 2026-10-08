---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: A new Alert arrives
    kind: condition
    unattended: true
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product walks the Route tree from the root to the deepest matching routes, going on to later siblings where a matching Route says to continue
    kind: product
    entities:
      - { entity: route, effect: reads, facts: [Matchers, Continue] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product starts an Alert group for each Route reached and the values of its grouping labels, holding the Alert
    kind: product
    entities:
      - { entity: alert-group, effect: creates, facts: [Group labels, Route labels, Receiver] }
      - { entity: alert, effect: reads, facts: [Labels] }
      - { entity: route, effect: reads, facts: [Group by, Receiver, Route labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The new Alert group waits for the group wait of its Route before it is first sent
    kind: condition
    entities:
      - { entity: alert-group, effect: reads, facts: [Group labels] }
      - { entity: route, effect: reads, facts: [Group wait] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Place an alert in a new group

## Trigger

An alert arrives that no existing alert group of its route holds.

## Outcome

The alert is in a new alert group that is notified once its group wait has passed.

## Edge cases

- An alert no deeper route matches is placed by the root route.
- Grouping by every label puts each alert in a group of its own; grouping by no labels puts every alert of the route in one group.
- A group created for an alert that started longer ago than the group wait is notified at once.
