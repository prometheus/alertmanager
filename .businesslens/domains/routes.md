---
colorSlot: 3
---

# Routes

Inspecting the routing tree and testing where a set of labels would be routed.

## Boundary

Owns the inspection of routes and the receivers they name. It does not own how
the configuration is written or loaded, nor the routing of live alerts, which
belongs to Alerts.
