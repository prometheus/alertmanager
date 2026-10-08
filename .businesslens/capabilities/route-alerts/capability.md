---
domain: alerts
availability:
  - { place: web-ui }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: dispatch/dispatch.go
  - kind: code
    role: implementation
    target: dispatch/route.go
  - kind: doc
    role: intent
    target: docs/alertmanager.md
---

# Route alerts

Alertmanager routes every alert it receives through the routing tree of its
configuration and places it in an alert group for each route it reaches, so
that alerts sharing the route's grouping labels are notified together.
