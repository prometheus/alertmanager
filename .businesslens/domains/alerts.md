---
colorSlot: 1
---

# Alerts

The alerts Alertmanager receives, how they are grouped, inhibited and
resolved, and the notifications sent for their groups.

## Boundary

Owns alerts, alert groups and the notifications sent for them. It does not own
silences, which belong to Silences, nor the routing tree, receivers and
inhibition rules, which the operator writes in the configuration.
