---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: An active Silence matches a firing Alert
    kind: condition
    unattended: true
    entities:
      - { entity: silence, effect: reads, facts: [Matchers] }
      - { entity: alert, effect: reads, facts: [Labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product marks the Alert suppressed and silenced by every active Silence matching it, so its Alert group leaves it out of what it sends
    kind: product
    entities:
      - { entity: alert, effect: changes, facts: [Status, Silenced by] }
      - { entity: silence, effect: reads, facts: [ID] }
      - { entity: alert-group, effect: reads, facts: [] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Mute an alert with an active silence

## Trigger

An alert fires that an active silence matches.

## Outcome

The alert is listed as silenced and is not notified.
