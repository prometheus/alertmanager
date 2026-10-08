---
kind: primary
routes:
  signal: Hang-up signal
  api: Reload endpoint
steps:
  - text: The Operator edits the file and asks for a reload of the Configuration
    kind: actor
    actor: operator
    entities:
      - { entity: configuration, effect: reads, facts: [] }
    contexts:
      signal:
        place: server
      api:
        place: api
  - text: The Product loads and checks the file and prepares its templates and integrations
    kind: product
    actor: operator
    entities: []
    contexts:
      signal:
        place: server
      api:
        place: api
  - text: The Product replaces the running Configuration with the new routes, receivers, inhibition rules and time intervals
    kind: product
    actor: operator
    entities:
      - { entity: configuration, effect: changes, facts: [Configuration text, Global settings, Notification templates, Event recorder outputs] }
      - { entity: route, effect: changes, facts: [Matchers, Receiver, Group by, Group wait, Group interval, Repeat interval, Continue, Mute time intervals, Active time intervals, Route labels] }
      - { entity: receiver, effect: changes, facts: [Name, Labels, Integrations, Send resolved] }
      - { entity: inhibition-rule, effect: changes, facts: [Name, Source matchers, Target matchers, Equal labels] }
      - { entity: time-interval, effect: changes, facts: [Name, Times, Weekdays, Days of month, Months, Years, Location] }
    contexts:
      signal:
        place: server
      api:
        place: api
  - text: The Product regroups every Alert it holds under the new routes, keeping silences and the notification log
    kind: product
    actor: operator
    entities:
      - { entity: alert-group, effect: changes, facts: [Group labels, Route labels, Receiver] }
      - { entity: alert, effect: reads, facts: [Labels] }
      - { entity: silence, effect: reads, facts: [] }
      - { entity: notification, effect: reads, facts: [] }
      - { entity: route, effect: reads, facts: [] }
    contexts:
      signal:
        place: server
      api:
        place: api
---

# Apply a new configuration

## Trigger

An operator has changed routing, receivers or rules in the configuration file.

## Outcome

Alertmanager runs with the new configuration; through the reload endpoint the request succeeds.

## Edge cases

- Receivers no route refers to are not set up.
- A receiver removed from the file is gone after the reload, and one added is available to routes at once.
- The web configuration file is not part of a reload; it is read on every request.
- A repeat interval longer than the data retention or shorter than the group interval is accepted with a warning in the log.
