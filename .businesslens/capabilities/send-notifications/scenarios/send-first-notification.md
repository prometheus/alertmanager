---
kind: primary
routes:
  server: Server
steps:
  - text: The group wait of a new Alert group elapses
    kind: condition
    unattended: true
    entities:
      - { entity: alert-group, effect: reads, facts: [Group labels] }
    contexts:
      server:
        place: server
  - text: The Product leaves out every muted Alert and renders the message from the templates of the Receiver with the group's labels, route labels and remaining alerts
    kind: product
    entities:
      - { entity: receiver, effect: reads, facts: [Name, Integrations] }
      - { entity: alert-group, effect: reads, facts: [Group labels, Route labels] }
      - { entity: alert, effect: reads, facts: [Labels, Annotations, Starts at, Ends at, Generator URL, Status] }
      - { entity: configuration, effect: reads, facts: [Notification templates] }
      - { entity: route, effect: reads, facts: [Route labels] }
    contexts:
      server:
        place: server
  - text: The Product sends a Notification through each integration of the Receiver
    kind: product
    entities:
      - { entity: receiver, effect: reads, facts: [Integrations] }
      - { entity: notification, effect: creates, to: Pending, facts: [Integration, Firing alerts, Resolved alerts] }
    contexts:
      server:
        place: server
  - text: Each integration accepts its Notification, and the Product records it in the notification log
    kind: product
    entities:
      - { entity: notification, effect: changes, from: Pending, to: Sent, facts: [] }
    contexts:
      server:
        place: server
---

# Send the first notification of a group

## Trigger

A new alert group's group wait passes.

## Outcome

Each integration of the receiver has one notification covering every firing alert in the group that nothing mutes.

## Edge cases

- While the instance is still settling into its cluster, notifications wait until it is ready.
- A group with nothing firing sends no first notification.
