---
kind: primary
routes:
  server: Server
steps:
  - text: The Operator starts Alertmanager with a config file, a storage path and peers to join
    kind: actor
    actor: operator
    entities:
      - { entity: peer, effect: reads, facts: [Address] }
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
  - text: The Product restores silences and the notification log from the storage path
    kind: product
    actor: operator
    entities:
      - { entity: silence, effect: reads, facts: [] }
      - { entity: notification, effect: reads, facts: [] }
    contexts:
      server:
        place: server
  - text: The Product joins its peers and settles into the cluster
    kind: product
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: creates, to: Settling, facts: [Version information, Uptime, Cluster name] }
      - { entity: peer, effect: reads, facts: [Name, Address] }
    contexts:
      server:
        place: server
  - text: Once the number of peers stays stable, or the settle timeout passes, the Alertmanager instance is ready and starts sending what is due
    kind: condition
    entities:
      - { entity: alertmanager-instance, effect: changes, from: Settling, to: Ready, facts: [] }
      - { entity: peer, effect: reads, facts: [] }
    contexts:
      server:
        place: server
---

# Start in a cluster

## Trigger

An operator starts Alertmanager as one of several clustered instances.

## Outcome

Alertmanager receives alerts and serves its web UI and API at once, and sends notifications once the cluster has settled.
