---
kind: primary
routes:
  web: Web UI
  api: API
steps:
  - text: The Operator asks for the current Alert groups
    kind: actor
    actor: operator
    entities:
      - { entity: alert-group, effect: reads, facts: [Group labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product works out for each unresolved Alert which silences and inhibiting alerts mute it now
    kind: product
    actor: operator
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Status, Silenced by, Inhibited by] }
      - { entity: silence, effect: reads, facts: [Matchers] }
      - { entity: inhibition-rule, effect: reads, facts: [Source matchers, Target matchers, Equal labels] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
  - text: The Product lists each Alert group with its group labels, Receiver and what mutes it, and the unresolved alerts it holds with their labels, annotations, start time, generator URL and what mutes them
    kind: product
    actor: operator
    entities:
      - { entity: alert-group, effect: reads, facts: [Group labels, Route labels, Receiver, Muted by] }
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at, Generator URL, Status, Silenced by, Inhibited by] }
      - { entity: receiver, effect: reads, facts: [Name] }
    contexts:
      web:
        place: web-ui::alerts
      api:
        place: api
---

# Browse alert groups

## Trigger

An operator wants to see what is firing.

## Outcome

The operator sees every alert group and the unresolved alerts in it.

## Edge cases

- In the web UI the operator may group alerts by labels of their choosing instead of by alert group.
- The web UI remembers in the browser whether all groups are shown expanded.
- Groups whose alerts are all silenced or inhibited can be left out.
