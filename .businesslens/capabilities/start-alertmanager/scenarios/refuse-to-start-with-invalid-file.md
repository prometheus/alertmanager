---
kind: validation
routes:
  server: Server
steps:
  - text: The Operator starts Alertmanager with a config file that fails its checks
    kind: actor
    actor: operator
    entities: []
    contexts:
      server:
        place: server
  - text: The Product exits with an error naming the problem
    kind: product
    actor: operator
    entities: []
    contexts:
      server:
        place: server
---

# Refuse to start with an invalid file

## Trigger

The configuration file has a mistake, such as a route naming an undefined receiver.

## Outcome

Alertmanager does not start and the operator is told why.
