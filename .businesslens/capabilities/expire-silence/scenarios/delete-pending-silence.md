---
kind: alternative
routes:
  web-list: Web UI silence list
  web-page: Web UI silence page
  api: API
  amtool: amtool
steps:
  - text: The Operator deletes a pending Silence and confirms
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Starts at] }
    contexts:
      web-list:
        place: web-ui::silences
      web-page:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product expires the Silence before it ever starts
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: changes, from: Pending, to: Expired, facts: [Starts at, Ends at, Updated at] }
    contexts:
      web-list:
        place: web-ui::silences
      web-page:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
---

# Delete a pending silence

## Trigger

Planned maintenance is cancelled.

## Outcome

The silence is expired without having muted anything.
