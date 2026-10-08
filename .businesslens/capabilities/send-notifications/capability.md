---
domain: alerts
availability:
  - { place: server }
references:
  - kind: code
    role: implementation
    target: notify/notify.go
  - kind: code
    role: implementation
    target: notify/dedup_stage.go
  - kind: code
    role: implementation
    target: notify/retry_stage.go
  - kind: code
    role: implementation
    target: notify/mute.go
  - kind: code
    role: implementation
    target: notify/cluster_stages.go
  - kind: code
    role: implementation
    target: nflog/nflog.go
  - kind: code
    role: implementation
    target: template/template.go
  - kind: doc
    role: intent
    target: docs/notifications.md
---

# Send notifications

Alertmanager notifies each alert group through every integration of its
receiver: first after the group wait, then whenever alerts join or resolve at
each group interval, and again after the repeat interval while nothing
changes. Each notification is rendered from the receiver's templates and
recorded in the notification log, which every peer shares. Operators follow
delivery in the logs and metrics of the server.
