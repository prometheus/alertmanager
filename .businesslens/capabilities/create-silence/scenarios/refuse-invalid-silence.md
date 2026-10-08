---
kind: validation
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator submits no matchers, only matchers that also match an empty label, an end not after the start, or an end already past
    kind: actor
    actor: operator
    entities: []
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product refuses the request with the reason and keeps nothing
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

# Refuse an invalid silence

## Trigger

An operator submits a silence that would match everything or never be active.

## Outcome

No silence is created and the operator is told why.

## Edge cases

- amtool refuses a zero duration, a start after the end and a missing comment before sending anything.
