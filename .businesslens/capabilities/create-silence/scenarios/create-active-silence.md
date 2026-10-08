---
kind: primary
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator writes matchers, a start, an end or a duration, a creator and a comment
    kind: actor
    actor: operator
    entities: []
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product checks the matchers and times and keeps a new active Silence under a new ID
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: creates, to: Active, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment, Annotations, Updated at] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
  - text: The Alert list shows the alerts the Silence matches as silenced from now on
    kind: condition
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [Matchers] }
      - { entity: alert, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::silence-editor
      api:
        place: api
      amtool:
        place: amtool
---

# Create an active silence

## Trigger

An operator knows about a problem and wants to stop being notified about it for a while.

## Outcome

The new silence is active and its ID is returned; the web UI shows the new silence.

## Edge cases

- A start in the past becomes the time the silence is created.
- amtool names the current system user as creator, lasts one hour and requires a comment unless told otherwise.
