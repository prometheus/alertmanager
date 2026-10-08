---
kind: alternative
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator writes matchers, a start in the future and an end
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
  - text: The Product keeps a new pending Silence under a new ID
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: creates, to: Pending, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
---

# Schedule a silence

## Trigger

An operator plans maintenance and wants its alerts silenced while it runs.

## Outcome

The silence is pending and starts muting at its start time.
