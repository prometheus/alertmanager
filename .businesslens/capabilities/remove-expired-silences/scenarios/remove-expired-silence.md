---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: Maintenance runs after the data retention period has passed since an expired Silence ended
    kind: condition
    unattended: true
    entities:
      - { entity: silence, effect: reads, facts: [Ends at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
  - text: The Product removes the Silence
    kind: product
    entities:
      - { entity: silence, effect: removes, from: Expired }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
---

# Remove an expired silence

## Trigger

Maintenance runs, every fifteen minutes by default, after the retention period of an expired silence, five days by default.

## Outcome

The expired silence is no longer listed and can no longer be recreated from the list.
