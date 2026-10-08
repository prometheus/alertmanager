---
availability:
  - { place: server }
  - { place: api }
references:
  - kind: code
    role: implementation
    target: app/reloader.go
  - kind: code
    role: implementation
    target: config/coordinator.go#Coordinator.Reload
  - kind: code
    role: implementation
    target: httpserver/httpserver.go#Register
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# Reload configuration

An operator makes a running Alertmanager pick up an edited configuration file,
with a hang-up signal or through the reload endpoint, without losing the
alerts, silences or notification log it holds.
