---
kind: edge
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator submits a new Silence while the maximum number of silences is reached, or one larger than the maximum silence size
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product refuses the request with the limit that was hit and keeps nothing
    kind: product
    actor: operator
    entities: []
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
---

# Refuse a silence over the limits

## Trigger

The operator has limited the number or size of silences and a new silence goes over it.

## Outcome

No silence is created and the operator is told which limit was reached.
