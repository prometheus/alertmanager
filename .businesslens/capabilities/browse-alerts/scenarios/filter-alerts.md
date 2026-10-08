---
kind: alternative
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator narrows the list with label matchers, a receiver pattern and whether silenced, inhibited and active ones are included
    kind: actor
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels] }
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product lists only the matching unresolved alerts with what mutes them now
    kind: product
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at, Receivers, Status, Silenced by, Inhibited by] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
      amtool:
        place: amtool
---

# Filter alerts

## Trigger

An operator looks for particular alerts.

## Outcome

Only the alerts the operator asked for are listed.

## Edge cases

- A matcher or receiver pattern that cannot be parsed is refused as a bad request.
- amtool lists only active alerts unless the operator asks for silenced or inhibited ones too.
- In amtool a first argument that is not a matcher is read as an alert name.
- No matching alert is a valid, empty answer.
