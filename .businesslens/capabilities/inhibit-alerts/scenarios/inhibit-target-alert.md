---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: A firing Alert matches the source matchers of an Inhibition rule
    kind: condition
    unattended: true
    entities:
      - { entity: alert, as: source, effect: reads, facts: [Labels] }
      - { entity: inhibition-rule, effect: reads, facts: [Source matchers] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: Another firing Alert matches the target matchers of that Inhibition rule and shares the values of its equal labels
    kind: condition
    entities:
      - { entity: alert, as: target, effect: reads, facts: [Labels] }
      - { entity: inhibition-rule, effect: reads, facts: [Target matchers, Equal labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product marks the target Alert suppressed and inhibited by the source, so its Alert group leaves it out of what it sends
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

# Inhibit a target alert

## Trigger

An alert covering a wider problem fires, such as a whole cluster being unreachable.

## Outcome

The narrower alerts it inhibits stay listed as inhibited but are not notified.

## Edge cases

- An alert that matches both sides of a rule is never inhibited by a source alert that also matches the target side.
- A label missing from both alerts counts as equal.
