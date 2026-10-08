---
domain: alerts
relations:
  - entity: alert
    verb: holds
    cardinality: many-to-many
  - entity: notification
    verb: is notified through
    cardinality: one-to-many
references:
  - kind: code
    role: implementation
    target: dispatch/dispatch.go
  - kind: doc
    role: intent
    target: docs/alertmanager.md
---

# Alert group

Alerts that matched the same route and share the values of that route's
grouping labels. Each alert group is notified as one unit, so a single
notification covers every alert in it. One alert belongs to one group for each
route it matches.

## Information kept

- **Group labels** — the values of the route's grouping labels shared by every alert in the group
- **Route labels** — the labels the route attaches, rendered for this group
- **Receiver** — the receiver the group is notified through
- **Muted by** — the time intervals currently muting the group
