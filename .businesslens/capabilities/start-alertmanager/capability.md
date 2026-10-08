---
availability:
  - { place: server }
references:
  - kind: code
    role: implementation
    target: cmd/alertmanager/main.go
  - kind: code
    role: implementation
    target: app/app.go
  - kind: code
    role: implementation
    target: app/lifecycle.go
  - kind: code
    role: implementation
    target: cluster/cluster.go
  - kind: doc
    role: intent
    target: docs/high_availability.md
---

# Start Alertmanager

An operator starts Alertmanager with a configuration file and command-line
settings. It loads and checks the configuration, restores silences and the
notification log from its storage path and, unless clustering is turned
off, joins its peers and settles into the cluster before sending
notifications.
