---
kind: primary
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator changes the end or the comment of an active Silence, keeping its matchers and start
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Matchers, Starts at, Ends at, Comment] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product updates the Silence in place under the same ID
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: changes, facts: [Ends at, Comment, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
---

# Extend a silence

## Trigger

Maintenance takes longer than planned.

## Outcome

The same silence, under the same ID, now ends later.

## Edge cases

- amtool sets the new end from the existing start plus the duration given.
- A pending silence whose new start is still in the future is also changed in place.
