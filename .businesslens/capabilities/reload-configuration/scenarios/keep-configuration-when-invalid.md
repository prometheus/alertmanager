---
kind: validation
routes:
  signal: Hang-up signal
  api: Reload endpoint
steps:
  - text: The Operator asks for a reload after an edit that leaves the file invalid
    kind: actor
    actor: operator
    entities: []
    contexts:
      signal:
        place: server
      api:
        place: api
  - text: The Product keeps running with the current Configuration and logs why the file was refused
    kind: product
    actor: operator
    entities:
      - { entity: configuration, effect: reads, facts: [Configuration text] }
    contexts:
      signal:
        place: server
      api:
        place: api
  - text: Through the reload endpoint the request fails with the reason, and the server metrics mark the last reload as failed
    kind: condition
    entities: []
    contexts:
      signal:
        place: server
      api:
        place: api
---

# Keep the configuration when the file is invalid

## Trigger

An operator reloads a configuration file with a mistake in it.

## Outcome

Nothing changes in the running Alertmanager and the operator can see the reload failed.
