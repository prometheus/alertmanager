---
domain: routes
relations:
  - entity: alert-group
    verb: groups alerts into
    cardinality: one-to-many
references:
  - kind: code
    role: implementation
    target: dispatch/route.go
  - kind: code
    role: implementation
    target: config/config.go#Route
---

# Route

A node of the routing tree. An alert enters at the root route and travels
down to the deepest routes whose matchers it satisfies; each route says which
receiver its alerts go to and how they are grouped and timed.

## Information kept

- **Matchers** — the label matchers an alert must satisfy to enter the route; the root route has none
- **Receiver** — the receiver notified for the route's alert groups
- **Group by** — the labels alerts are grouped by, or every label
- **Group wait** — how long a new alert group waits before its first notification
- **Group interval** — how long to wait before notifying about alerts added to a group already notified
- **Repeat interval** — how long to wait before repeating an unchanged notification
- **Continue** — whether an alert matching this route goes on to match its later siblings
- **Mute time intervals** — the time intervals during which the route sends nothing
- **Active time intervals** — the time intervals outside which the route sends nothing
- **Route labels** — labels, possibly templated, attached to the route's groups and offered to notification templates
