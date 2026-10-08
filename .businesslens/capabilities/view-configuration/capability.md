---
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#getStatusHandler
  - kind: code
    role: implementation
    target: ui/app/src/Views/Status/Views.elm
  - kind: code
    role: implementation
    target: cli/config.go
  - kind: code
    role: implementation
    target: config/config.go
---

# View configuration

Operators read the configuration a running Alertmanager uses, as YAML with
every secret hidden. It shows what Alertmanager loaded, not the file as
written.
