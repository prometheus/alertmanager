---
kind: alternative
routes:
  server: Server
steps:
  - text: Alerts of a notified Alert group have resolved by its next group interval
    kind: condition
    unattended: true
    entities:
      - { entity: alert, effect: reads, facts: [Ends at] }
      - { entity: alert-group, effect: reads, facts: [] }
    contexts:
      server:
        place: server
  - text: The Product sends a Notification of what resolved through each integration of the Receiver that sends resolved notifications
    kind: product
    entities:
      - { entity: receiver, effect: reads, facts: [Integrations, Send resolved] }
      - { entity: notification, effect: creates, to: Pending, facts: [Integration, Firing alerts, Resolved alerts] }
    contexts:
      server:
        place: server
  - text: Each integration accepts its Notification and the notification log records it
    kind: product
    entities:
      - { entity: notification, effect: changes, from: Pending, to: Sent, facts: [] }
    contexts:
      server:
        place: server
  - text: The Product drops the resolved alerts from the Alert group and removes the group once it holds none
    kind: product
    entities:
      - { entity: alert-group, effect: removes }
      - { entity: alert, effect: reads, facts: [] }
    contexts:
      server:
        place: server
---

# Send a resolved notification

## Trigger

Alerts in a notified group resolve.

## Outcome

Integrations that send resolved notifications know the alerts resolved, the others hear nothing, and an empty group is gone.

## Edge cases

- A group whose alerts resolved before it was ever notified sends nothing.
- An alert that was notified and then muted until it resolved gets no resolved notification.
