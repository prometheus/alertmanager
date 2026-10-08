---
kind: alternative
routes:
  server: Server
steps:
  - text: The group interval of an Alert group already notified elapses
    kind: condition
    unattended: true
    entities:
      - { entity: alert-group, effect: reads, facts: [Group labels] }
    contexts:
      server:
        place: server
  - text: The Product compares the firing and resolved alerts of the group with the last Notification the notification log holds for each integration
    kind: product
    entities:
      - { entity: alert, effect: reads, facts: [Fingerprint] }
      - { entity: notification, as: previous, effect: reads, facts: [Integration, Firing alerts, Resolved alerts] }
      - { entity: route, effect: reads, facts: [Repeat interval] }
    contexts:
      server:
        place: server
  - text: The Product sends a new Notification through each integration of the Receiver
    kind: product
    entities:
      - { entity: notification, as: next, effect: creates, to: Pending, facts: [Integration, Firing alerts, Resolved alerts] }
      - { entity: receiver, effect: reads, facts: [Integrations] }
    contexts:
      server:
        place: server
  - text: Each integration accepts the new Notification and the notification log records it
    kind: product
    entities:
      - { entity: notification, as: next, effect: changes, from: Pending, to: Sent, facts: [] }
    contexts:
      server:
        place: server
---

# Send a follow-up notification

## Trigger

An alert group's group interval passes after its first notification.

## Outcome

The receiver hears about the group again.

## Decision points

### Why is the group notified again?

Has anything changed since the last notification to this integration?

- New firing alerts joined the group → the notification is sent now with the new alerts.
- Nothing changed and the repeat interval has passed → the unchanged notification is repeated.

## Edge cases

- When nothing changed and the repeat interval has not passed, nothing is sent to that integration.
- Each integration is compared on its own, so one integration's failure does not resend to the others.
