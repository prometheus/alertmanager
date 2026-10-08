---
kind: primary
routes:
  web: Web UI
steps:
  - text: The Operator chooses the first day of the week
    kind: actor
    actor: operator
    entities: []
    contexts:
      web:
        place: web-ui::settings
  - text: The Product keeps the choice in the Browser settings of this browser
    kind: product
    actor: operator
    entities:
      - { entity: browser-settings, effect: changes, facts: [First day of the week] }
    contexts:
      web:
        place: web-ui::settings
---

# Choose the first day of the week

## Trigger

An operator wants the silence editor's calendar to start weeks on their usual day.

## Outcome

The silence editor's calendar starts weeks on the chosen day in this browser.

## Edge cases

- Until a day is chosen, weeks start on Sunday.
