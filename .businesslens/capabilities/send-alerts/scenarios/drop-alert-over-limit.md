---
kind: edge
routes:
  api: API
steps:
  - text: The Alert generator sends a new Alert under a name that already has as many unresolved alerts as the per-alert-name limit allows
    kind: actor
    actor: alert-generator
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Ends at] }
    contexts:
      api:
        place: api
  - text: The Product drops the new Alert, still accepting refreshes of the alerts it already holds under that name
    kind: product
    actor: alert-generator
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      api:
        place: api
  - text: The request succeeds and the drop is only counted in the metrics of the Alertmanager instance
    kind: condition
    actor: alert-generator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Metrics] }
    contexts:
      api:
        place: api
---

# Drop an alert over the per-name limit

## Trigger

Far more alerts than expected arrive with the same alert name while the
operator has set a per-alert-name limit.

## Outcome

No new alert is kept for that name until room frees up as held alerts resolve;
the receivers are not flooded.
