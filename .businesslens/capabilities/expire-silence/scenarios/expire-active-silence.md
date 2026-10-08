---
kind: primary
routes:
  web-list: Web UI silence list
  web-page: Web UI silence page
  api: API
  amtool: amtool
steps:
  - text: The Operator expires an active Silence and confirms
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Matchers, Ends at] }
    contexts:
      web-list:
        place: web-ui::silences
      web-page:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product ends the Silence now
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: changes, from: Active, to: Expired, facts: [Ends at, Updated at] }
    contexts:
      web-list:
        place: web-ui::silences
      web-page:
        place: web-ui::silence
      api:
        place: api
      amtool:
        place: amtool
  - text: Alerts it muted are listed as active again unless something else mutes them
    kind: condition
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [] }
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

# Expire an active silence

## Trigger

The problem a silence covered is fixed early, or the silence was a mistake.

## Outcome

The silence is expired and no longer mutes anything.

## Edge cases

- Expiring a silence that has already expired changes nothing and succeeds.
- amtool expires the IDs given in order and stops at the first one that fails.
