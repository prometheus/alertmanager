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
    target: cli/cluster.go
---

# View status

Operators see an Alertmanager's version information, uptime, cluster status
and peers.
