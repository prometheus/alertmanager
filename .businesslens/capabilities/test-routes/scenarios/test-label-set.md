---
kind: primary
routes:
  amtool: amtool
steps:
  - text: The Operator gives amtool a set of labels and a config file or a running Alertmanager
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: amtool matches the labels against the Route tree and prints the receivers they reach in tree order
    kind: product
    actor: operator
    entities:
      - { entity: route, effect: reads, facts: [Matchers, Continue, Receiver] }
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      amtool:
        place: amtool
---

# Test a label set

## Trigger

An operator wants to know where an alert would go before it fires.

## Outcome

The operator sees the receivers the labels reach.

## Edge cases

- amtool can also print the matching part of the routing tree.
