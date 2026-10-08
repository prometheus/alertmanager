---
kind: primary
routes:
  server: Server
steps:
  - text: An Alert is received for the first time, grouped or resolved, a Silence is created, updated or mutes an Alert, or a Notification is sent
    kind: condition
    unattended: true
    entities:
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at, Ends at, Fingerprint] }
      - { entity: silence, effect: reads, facts: [ID, Matchers, Starts at, Ends at, Created by, Comment] }
      - { entity: notification, effect: reads, facts: [Integration] }
    contexts:
      server:
        place: server
  - text: The Product sends an event naming this Alertmanager instance and its cluster position to every output of the Configuration
    kind: product
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Cluster name] }
      - { entity: configuration, effect: reads, facts: [Event recorder outputs] }
    contexts:
      server:
        place: server
---

# Record an event

## Trigger

Something significant happens while event recording is enabled.

## Outcome

Every configured output receives the event.

## Edge cases

- Webhook outputs send events in batches and retry failed batches a few times.
- Kafka outputs send JSON or protocol buffer messages.
