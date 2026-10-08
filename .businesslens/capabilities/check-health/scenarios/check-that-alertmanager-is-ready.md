---
kind: alternative
routes:
  api: API
steps:
  - text: The Operator asks the Alertmanager instance whether it is ready to serve traffic
    kind: actor
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [] }
    contexts:
      api:
        place: api
  - text: The Product answers that the Alertmanager instance is ready once it serves queries
    kind: product
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Readiness] }
    contexts:
      api:
        place: api
---

# Check that Alertmanager is ready

## Trigger

A load balancer or orchestrator decides whether to send traffic to the instance.

## Outcome

The check succeeds once Alertmanager answers queries; it does not wait for the cluster to settle.
