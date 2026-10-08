---
kind: edge
routes:
  server: Server
steps:
  - text: An integration of the Receiver fails to deliver a Notification with an error retrying can fix
    kind: condition
    unattended: true
    entities:
      - { entity: receiver, effect: reads, facts: [Integrations] }
      - { entity: notification, effect: reads, facts: [Integration] }
    contexts:
      server:
        place: server
  - text: The Product retries the Notification with a growing delay, honoring any delay the service asks for, within the group interval
    kind: product
    entities:
      - { entity: notification, effect: reads, facts: [Integration] }
    contexts:
      server:
        place: server
  - text: A retry is accepted and the notification log records the Notification
    kind: product
    entities:
      - { entity: notification, effect: changes, from: Pending, to: Sent, facts: [] }
    contexts:
      server:
        place: server
---

# Retry a failed notification

## Trigger

A notification service is briefly unreachable or asks Alertmanager to try later.

## Outcome

The notification is delivered by a later attempt and the failed attempts are counted in the server metrics.
