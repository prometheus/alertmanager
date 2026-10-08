---
kind: primary
routes:
  server: Server
steps:
  - text: The Operator sends Alertmanager a termination signal
    kind: actor
    actor: operator
    entities: []
    contexts:
      server:
        place: server
  - text: The Product stops serving and notifying, leaves the cluster and writes silences and the notification log to disk
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
      - { entity: notification, effect: reads, facts: [] }
      - { entity: alertmanager-instance, effect: removes, from: Ready }
    contexts:
      server:
        place: server
---

# Stop gracefully

## Trigger

An operator shuts Alertmanager down for maintenance or an upgrade.

## Outcome

Alertmanager has exited; its silences and notification log are on disk for the next start, and the alerts it held are gone.

## Edge cases

- A stand-alone Alertmanager stops the same way, without a cluster to leave.
- A failure to write the silences or the notification log is reported in the log.
