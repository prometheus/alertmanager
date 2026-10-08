---
kind: alternative
routes:
  amtool: amtool
steps:
  - text: The Operator runs amtool's add command with labels, annotations and optional start and end times
    kind: actor
    actor: operator
    entities: []
    contexts:
      amtool:
        place: amtool
  - text: The Product keeps a new firing Alert identified by its labels
    kind: product
    actor: operator
    entities:
      - { entity: alert, effect: creates, to: Firing, facts: [Labels, Annotations, Starts at, Ends at, Updated at, Generator URL, Fingerprint] }
    contexts:
      amtool:
        place: amtool
---

# Add an alert with amtool

## Trigger

An operator wants to see how Alertmanager routes and notifies a particular alert.

## Outcome

Alertmanager holds the alert just as if an alert generator had sent it.

## Edge cases

- A first argument that is not a label pair is taken as the alert name.
- Labels and annotations other than name=value pairs are refused before anything is sent.
