---
kind: alternative
routes:
  server: Server
steps:
  - text: An Alert group is due to be sent while the time is inside a mute Time interval of its Route, or outside all of its active time intervals
    kind: condition
    unattended: true
    entities:
      - { entity: alert-group, effect: reads, facts: [] }
      - { entity: route, effect: reads, facts: [Mute time intervals, Active time intervals] }
      - { entity: time-interval, effect: reads, facts: [Times, Weekdays, Days of month, Months, Years, Location] }
    contexts:
      server:
        place: server
  - text: The Product records the time intervals muting the Alert group and sends nothing
    kind: product
    entities:
      - { entity: alert-group, effect: changes, facts: [Muted by] }
      - { entity: time-interval, effect: reads, facts: [Name] }
    contexts:
      server:
        place: server
---

# Mute a group by time interval

## Trigger

An alert group is due for a notification outside the hours its route notifies in.

## Outcome

Nothing is sent, and the group shows which time intervals mute it.

## Edge cases

- A route only uses the time intervals it names itself; a child route never inherits its parent's.
- A route with no active time intervals is always active.
