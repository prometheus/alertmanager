---
kind: primary
routes:
  amtool: amtool
steps:
  - text: The Operator imports a file of silences exported by amtool
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
    contexts:
      amtool:
        place: amtool
  - text: The Product keeps each Silence, several at a time, recreating any whose ID it does not know under a new ID
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      amtool:
        place: amtool
  - text: amtool prints the ID of every Silence kept
    kind: condition
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [ID] }
    contexts:
      amtool:
        place: amtool
---

# Import silences from a file

## Trigger

An operator moves silences to another Alertmanager or restores them.

## Outcome

Every silence in the file exists in the Alertmanager.

## Edge cases

- A silence whose ID the Alertmanager knows is submitted as an edit of that silence.
- Forcing the import drops every ID, so each silence is created anew.
