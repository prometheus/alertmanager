---
kind: alternative
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator opens one Silence by its ID
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [ID] }
    contexts:
      web:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product shows the Silence's matchers, times, creator, comment, annotations and state, and the alerts it currently affects
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at] }
    contexts:
      web:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
---

# View a silence

## Trigger

An operator follows a link to a silence, or wants to check what one mutes.

## Outcome

The operator sees the silence and the alerts it affects.

## Edge cases

- An unknown ID is answered as not found.
