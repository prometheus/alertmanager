---
kind: alternative
routes:
  web: Web UI
steps:
  - text: The Operator chooses to recreate an expired Silence
    kind: actor
    actor: operator
    entities:
      - { entity: silence, as: expired, effect: reads, facts: [Matchers] }
    contexts:
      web:
        place: web-ui::silences
  - text: The Product opens the editor with the matchers and comment of the expired Silence
    kind: product
    actor: operator
    entities:
      - { entity: silence, as: expired, effect: reads, facts: [Matchers, Comment] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Operator sets a new start and end and creates it
    kind: actor
    actor: operator
    entities: []
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product keeps a new active Silence under a new ID, leaving the expired one as it was
    kind: product
    actor: operator
    entities:
      - { entity: silence, as: new, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
      - { entity: silence, as: expired, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::silence-editor
---

# Recreate an expired silence

## Trigger

A problem that was silenced before is back.

## Outcome

A new active silence with the same matchers exists beside the expired one.
