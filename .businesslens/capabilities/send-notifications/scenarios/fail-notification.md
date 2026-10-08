---
kind: edge
routes:
  server: Server
steps:
  - text: An integration refuses a Notification with an error retrying cannot fix, or retries run past the group interval
    kind: condition
    unattended: true
    entities:
      - { entity: notification, effect: reads, facts: [Integration] }
    contexts:
      server:
        place: server
  - text: The Product gives up on the Notification and logs and counts the failure in the server metrics
    kind: product
    entities:
      - { entity: notification, effect: changes, from: Pending, to: Failed, facts: [Failure] }
    contexts:
      server:
        place: server
---

# Give up on a notification

## Trigger

A notification service keeps refusing or stays unreachable.

## Outcome

Nothing is recorded as sent for that integration, so the group is tried again at its next group interval; the operator sees the failure in the logs and metrics.
