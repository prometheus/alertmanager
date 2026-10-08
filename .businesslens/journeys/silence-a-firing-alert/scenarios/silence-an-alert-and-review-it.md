---
kind: primary
result: achieved
routes:
  web: Web UI
steps:
  - text: The Operator asks to quiet an Alert from the alert list
    kind: actor
    actor: operator
    capability: create-silence
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::alerts
  - text: The Operator sets the duration, creator and comment in the editor and creates the silence
    kind: actor
    actor: operator
    capability: create-silence
    entities:
      - { entity: silence, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product keeps a new active Silence, remembers its creator in the Browser settings and takes the Operator to its page
    kind: product
    actor: operator
    capability: create-silence
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
      - { entity: browser-settings, effect: changes, facts: [Default creator] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product shows the Silence and the alerts it now affects
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

# Silence an alert and review it

## Trigger

An operator looking at the alert list recognises an alert they are already handling.

## Outcome

The operator is on the page of the new silence and sees the alerts it mutes.
