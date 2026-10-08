---
kind: alternative
routes:
  web: Web UI
  api: API
steps:
  - text: A Peer gossips a Silence this Alertmanager knows, updated more recently than its own copy
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
  - text: The Product replaces its copy of the Silence with the newer one
    kind: product
    entities:
      - { entity: silence, effect: changes, facts: [Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      web:
        place: web-ui::silences
      api:
        place: api
---

# Take a newer silence from a peer

## Trigger

An operator edits or expires a silence on another Alertmanager of the cluster.

## Outcome

This Alertmanager lists and applies the newer version of the silence.

## Edge cases

- A copy updated at the same time or earlier than the one held changes nothing.
