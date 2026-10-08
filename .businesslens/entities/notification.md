---
domain: alerts
references:
  - kind: code
    role: implementation
    target: notify/notify.go
  - kind: code
    role: implementation
    target: notify/retry_stage.go
  - kind: code
    role: implementation
    target: nflog/nflog.go
  - kind: doc
    role: intent
    target: docs/notifications.md
---

# Notification

One message about an alert group sent through one integration of its
receiver. Alertmanager records each notification sent in its notification log,
which it shares with every peer and keeps for the data retention period, and
compares later notifications against it.

## Information kept

- **Integration** — the receiver integration it goes through, such as one Slack channel or one webhook
- **Firing alerts** — the firing alerts it covers
- **Resolved alerts** — the resolved alerts it covers
- **Failure** — why delivery failed, when it did

## States

### Pending

Due, and being delivered or retried.

### Sent

The integration accepted it; it is recorded in the notification log.

### Failed

Delivery failed in a way retrying cannot fix, or retries ran out of time.
