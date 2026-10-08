---
kind: edge
routes:
  amtool: amtool
steps:
  - text: The Operator imports a file in which some silences are refused
    kind: actor
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
    contexts:
      amtool:
        place: amtool
  - text: The Product keeps every acceptable Silence and reports each refusal without stopping
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      amtool:
        place: amtool
  - text: amtool reports how many silences could not be imported and exits with an error
    kind: condition
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
    contexts:
      amtool:
        place: amtool
---

# Report failed imports

## Trigger

Some silences in an import are invalid or over a limit.

## Outcome

The acceptable silences are kept and the operator knows how many failed.

## Edge cases

- Input that is not a JSON list of silences is refused before anything is imported.
