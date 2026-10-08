---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: A Peer gossips a Silence this Alertmanager does not know, whose retention has not run out
    kind: condition
    unattended: true
    entities:
      - { entity: peer, effect: reads, facts: [Name] }
      - { entity: silence, effect: reads, facts: [ID, Updated at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
  - text: The Product keeps the Silence as it arrived
    kind: product
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
---

# Receive a silence from a peer

## Trigger

An operator creates a silence on another Alertmanager of the cluster.

## Outcome

The silence is listed here too and mutes the alerts it matches here.

## Edge cases

- A pending or expired silence arrives in the same state it has on the peer.
- A silence received from a peer is kept whatever the local limits on the number and size of silences.
