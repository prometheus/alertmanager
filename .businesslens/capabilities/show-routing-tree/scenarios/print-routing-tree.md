---
kind: primary
routes:
  amtool: amtool
steps:
  - text: The Operator asks amtool for the routing tree of a config file or of a running Alertmanager
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: amtool prints each Route with its matchers, whether it continues and its Receiver
    kind: product
    actor: operator
    entities:
      - { entity: route, effect: reads, facts: [Matchers, Continue, Receiver] }
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      amtool:
        place: amtool
---

# Print the routing tree

## Trigger

An operator wants to understand how alerts are routed.

## Outcome

The operator sees the routing tree.

## Edge cases

- Without a file or an Alertmanager address amtool refuses to run.
- A file given together with an address takes precedence, with a warning.
