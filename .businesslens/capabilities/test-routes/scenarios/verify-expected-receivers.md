---
kind: validation
routes:
  amtool: amtool
steps:
  - text: The Operator also names the receivers the labels are expected to reach
    kind: actor
    actor: operator
    entities:
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      amtool:
        place: amtool
  - text: amtool warns that the expected and resolved receivers differ and exits with an error
    kind: product
    actor: operator
    entities:
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      amtool:
        place: amtool
---

# Verify expected receivers

## Trigger

A routing test in a script or CI job expects particular receivers.

## Outcome

The test fails when routing does not send the labels where expected.

## Edge cases

- The expected receivers must be listed in the same order as the routing tree resolves them.
