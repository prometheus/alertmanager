---
kind: primary
routes:
  api: API
steps:
  - text: The Alert generator sends what fired with its labels, annotations and a generator URL
    kind: actor
    actor: alert-generator
    entities: []
    contexts:
      api:
        place: api
  - text: The Product fills a missing start time with the time of receipt and a missing end time with the receipt time plus the resolve timeout of the Configuration
    kind: product
    actor: alert-generator
    entities:
      - { entity: configuration, effect: reads, facts: [Global settings] }
    contexts:
      api:
        place: api
  - text: The Product keeps a new firing Alert identified by its labels
    kind: product
    actor: alert-generator
    entities:
      - { entity: alert, effect: creates, to: Firing, facts: [Labels, Annotations, Starts at, Ends at, Updated at, Generator URL, Fingerprint] }
    contexts:
      api:
        place: api
---

# Send a firing alert

## Trigger

An alerting rule in the alert generator starts firing.

## Outcome

Alertmanager holds the new alert and the request succeeds.

## Edge cases

- Labels with empty values are dropped before the alert is kept.
- A missing start time becomes the end time when only an end time is sent.
- An alert sent already resolved is kept as resolved.
