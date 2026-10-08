---
kind: validation
routes:
  api: API
  amtool: amtool
steps:
  - text: The Operator expires an ID no Silence has
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [ID] }
    contexts:
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product refuses the request as not found
    kind: product
    actor: operator
    entities: []
    contexts:
      api:
        place: api
      amtool:
        place: amtool
---

# Refuse expiry of an unknown silence

## Trigger

An operator expires a silence that does not exist or was already removed.

## Outcome

Nothing changes and the operator is told the silence was not found.
