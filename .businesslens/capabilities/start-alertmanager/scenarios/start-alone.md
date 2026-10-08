---
kind: alternative
routes:
  server: Server
steps:
  - text: The Operator starts Alertmanager with clustering turned off
    kind: actor
    actor: operator
    entities: []
    contexts:
      server:
        place: server
  - text: The Product loads and checks the file and runs with its Configuration, routes, receivers, inhibition rules and time intervals
    kind: product
    actor: operator
    entities:
      - { entity: configuration, effect: creates, facts: [Configuration text, Global settings, Notification templates, Event recorder outputs] }
      - { entity: route, effect: creates, facts: [Matchers, Receiver, Group by, Group wait, Group interval, Repeat interval, Continue, Mute time intervals, Active time intervals, Route labels], with: configuration }
      - { entity: receiver, effect: creates, facts: [Name, Labels, Integrations, Send resolved], with: configuration }
      - { entity: inhibition-rule, effect: creates, facts: [Name, Source matchers, Target matchers, Equal labels], with: configuration }
      - { entity: time-interval, effect: creates, facts: [Name, Times, Weekdays, Days of month, Months, Years, Location], with: configuration }
    contexts:
      server:
        place: server
  - text: The Product restores silences and the notification log and runs without peers, reporting clustering as disabled
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
      - { entity: notification, effect: reads, facts: [] }
      - { entity: alertmanager-instance, effect: creates, to: Disabled, facts: [Version information, Uptime, Cluster name] }
      - { entity: peer, effect: reads, facts: [] }
    contexts:
      server:
        place: server
---

# Start alone

## Trigger

An operator runs a single Alertmanager.

## Outcome

Alertmanager runs on its own and notifies without waiting for peers.
