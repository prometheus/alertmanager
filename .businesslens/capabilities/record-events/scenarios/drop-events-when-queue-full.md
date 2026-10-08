---
kind: edge
routes:
  server: Server
steps:
  - text: Events arrive faster than the outputs take them and the event queue is full
    kind: condition
    unattended: true
    entities: []
    contexts:
      server:
        place: server
  - text: The Product drops the new events and counts them in the server metrics
    kind: product
    entities: []
    contexts:
      server:
        place: server
---

# Drop events when the queue is full

## Trigger

An output is slow or unreachable during a burst of events.

## Outcome

Alerting goes on unaffected and the operator can see that events were dropped.
