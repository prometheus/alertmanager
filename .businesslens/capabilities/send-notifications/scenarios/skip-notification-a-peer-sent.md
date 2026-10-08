---
kind: alternative
routes:
  server: Server
steps:
  - text: An Alert group is due to be sent while clustering is on
    kind: condition
    unattended: true
    entities:
      - { entity: alert-group, effect: reads, facts: [] }
    contexts:
      server:
        place: server
  - text: The Product waits one peer timeout for every Peer ordered before it in the cluster
    kind: product
    entities:
      - { entity: peer, effect: reads, facts: [Name] }
    contexts:
      server:
        place: server
  - text: The Product finds that a Peer has already recorded the Notification in the shared notification log and sends nothing
    kind: product
    entities:
      - { entity: peer, effect: reads, facts: [] }
      - { entity: notification, effect: reads, facts: [Integration, Firing alerts, Resolved alerts] }
    contexts:
      server:
        place: server
---

# Skip a notification a peer sent

## Trigger

Several clustered Alertmanagers hold the same alert group.

## Outcome

The receiver gets the notification once, from the first peer that could send it.

## Edge cases

- While peers cannot reach each other, each side notifies, so receivers may get duplicates rather than nothing.
