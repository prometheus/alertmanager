---
availability:
  - { place: server }
references:
  - kind: code
    role: implementation
    target: app/lifecycle.go
  - kind: code
    role: implementation
    target: app/app.go
---

# Stop Alertmanager

An operator stops a running Alertmanager with a termination signal. It stops
serving and notifying, leaves its cluster and writes its silences and
notification log to disk one last time.
