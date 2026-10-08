---
kind: primary
result: achieved
routes:
  web: Web UI
steps:
  - text: The Operator changes the end of an active Silence in the editor and updates it
    kind: actor
    actor: operator
    capability: edit-silence
    entities:
      - { entity: silence, effect: reads, facts: [Ends at] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product updates the Silence and takes the Operator to its page
    kind: product
    actor: operator
    capability: edit-silence
    entities:
      - { entity: silence, effect: changes, facts: [Ends at, Comment, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product shows the Silence and the alerts it affects
    kind: product
    actor: operator
    capability: browse-silences
    entities:
      - { entity: silence, effect: reads, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at] }
    contexts:
      web:
        place: web-ui::silence
---

# Extend a silence and review it

## Trigger

An operator needs a silence to last longer.

## Outcome

The operator is on the page of the extended silence.
