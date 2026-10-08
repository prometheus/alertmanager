---
kind: primary
routes:
  api: API
steps:
  - text: The Operator asks the Alertmanager instance whether it is healthy
    kind: actor
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [] }
    contexts:
      api:
        place: api
  - text: The Product answers that the Alertmanager instance is healthy for as long as it serves HTTP
    kind: product
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Health] }
    contexts:
      api:
        place: api
---

# Check that Alertmanager is up

## Trigger

A liveness probe or an operator checks the instance.

## Outcome

The check succeeds while Alertmanager serves HTTP.
