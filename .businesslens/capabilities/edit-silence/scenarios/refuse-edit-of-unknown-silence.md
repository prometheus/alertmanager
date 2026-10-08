---
kind: validation
routes:
  api: API
  amtool: amtool
steps:
  - text: The Operator submits changes under an ID no Silence has
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [ID] }
    contexts:
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product refuses the request as not found and keeps nothing
    kind: product
    actor: operator
    entities: []
    contexts:
      api:
        place: api
      amtool:
        place: amtool
---

# Refuse an edit of an unknown silence

## Trigger

An operator edits a silence that never existed or has already been removed.

## Outcome

Nothing changes and the operator is told the silence was not found.
