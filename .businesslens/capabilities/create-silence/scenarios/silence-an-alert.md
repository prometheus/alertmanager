---
kind: alternative
routes:
  web: Web UI
steps:
  - text: The Operator asks to quiet an Alert from the alert list
    kind: actor
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::alerts
  - text: The Product opens the editor with one equality matcher for each label of the Alert
    kind: product
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Operator previews the alerts the matchers select and sets the duration, creator and comment
    kind: actor
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at] }
    contexts:
      web:
        place: web-ui::silence-editor
  - text: The Product keeps a new active Silence under a new ID and remembers its creator in the Browser settings
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
      - { entity: browser-settings, effect: changes, facts: [Default creator] }
    contexts:
      web:
        place: web-ui::silence-editor
---

# Silence an alert

## Trigger

An operator looking at the alert list wants to silence one alert.

## Outcome

A silence matching exactly that alert is active.

## Edge cases

- The filter bar of the alert list offers the same with the current filter matchers.
- The editor fills the creator with the one remembered in the browser settings.
