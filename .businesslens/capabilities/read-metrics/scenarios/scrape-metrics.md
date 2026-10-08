---
kind: primary
routes:
  api: API
steps:
  - text: The Operator's monitoring scrapes the metrics of the Alertmanager instance
    kind: actor
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [] }
    contexts:
      api:
        place: api
  - text: The Product returns the current metrics of the Alertmanager instance
    kind: product
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Metrics] }
    contexts:
      api:
        place: api
---

# Scrape metrics

## Trigger

A Prometheus server monitoring Alertmanager scrapes it.

## Outcome

The operator can graph and alert on how Alertmanager is doing, such as failed notifications or a failed reload.
