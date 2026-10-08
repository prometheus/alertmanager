---
kind: alternative
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator changes the matchers or the start of an active Silence
    kind: actor
    actor: operator
    entities:
      - { entity: silence, as: original, effect: reads, facts: [Matchers, Starts at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product keeps a new active Silence under a new ID with the changed values
    kind: product
    actor: operator
    entities:
      - { entity: silence, as: replacement, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product expires the original Silence
    kind: product
    actor: operator
    entities:
      - { entity: silence, as: original, effect: changes, from: Active, to: Expired, facts: [Ends at, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
---

# Replace a silence

## Trigger

A silence turns out to match the wrong alerts.

## Outcome

A new silence with the changed values is active and the original one has expired.

## Edge cases

- Changing an expired silence always keeps a new silence and leaves the expired one as it was.
- amtool cannot change matchers or the creator; it changes start, end, duration and comment.
