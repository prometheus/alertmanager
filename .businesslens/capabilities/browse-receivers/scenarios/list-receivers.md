---
kind: primary
routes:
  api: API
steps:
  - text: The Operator asks for the receivers, optionally narrowed by matchers on their labels
    kind: actor
    actor: operator
    entities:
      - { entity: receiver, effect: reads, facts: [Labels] }
    contexts:
      api:
        place: api
  - text: The Product lists each Receiver's name and labels
    kind: product
    actor: operator
    entities:
      - { entity: receiver, effect: reads, facts: [Name, Labels] }
    contexts:
      api:
        place: api
---

# List receivers

## Trigger

An operator or a tool needs the receivers alerts can be filtered by.

## Outcome

The receivers of the running configuration are listed.
