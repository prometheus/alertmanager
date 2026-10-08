---
domain: alerts
references:
  - kind: code
    role: implementation
    target: alert/alert.go
  - kind: code
    role: implementation
    target: provider/mem/mem.go
  - kind: code
    role: implementation
    target: api/v2/api.go#alertFilter
  - kind: doc
    role: intent
    target: docs/alerts_api.md
---

# Alert

One alert sent by an alert generator, identified by its labels. Alerts with
identical labels are the same alert: sending it again refreshes the alert
Alertmanager already holds.

## Information kept

- **Labels** — the label set that identifies the alert, such as alertname, cluster and instance
- **Annotations** — further information such as a summary, a description or a runbook link
- **Starts at** — when the alert started firing
- **Ends at** — when the alert resolves; while the alert generator keeps refreshing it this lies in the future
- **Updated at** — when Alertmanager last received the alert
- **Generator URL** — a link back to the source of the alert, such as the firing rule
- **Fingerprint** — the identifier derived from the labels
- **Receivers** — the receivers the routing tree sends the alert to
- **Status** — active, or suppressed while a silence or an inhibiting alert mutes it, worked out each time the alert is listed
- **Silenced by** — the active silences that mute the alert
- **Inhibited by** — the firing alerts that inhibit it

## States

### Firing

Its end time has not passed.

### Resolved

Its end time has passed, because the alert generator resolved it or stopped
refreshing it. A resolved alert is no longer listed; it is kept until resolved
notifications have gone out and the next alert clean-up removes it.
