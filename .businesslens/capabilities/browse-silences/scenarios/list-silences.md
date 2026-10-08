---
kind: primary
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator asks for the silences, optionally narrowed by label matchers
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Matchers] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product lists active silences ending soonest first, then pending ones, then expired ones most recent first
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Matchers, Starts at, Ends at, Annotations] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
      amtool:
        place: amtool
---

# List silences

## Trigger

An operator wants to know what is silenced.

## Outcome

The operator sees the silences in each state.

## Edge cases

- amtool hides expired silences unless asked, and can show only those ending or expired within a given time.
- amtool can print only the IDs, ready to be expired.
