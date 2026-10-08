---
entities:
  - entity: alertmanager-instance
    shows: [Version information, Uptime, Cluster name]
  - entity: peer
    shows: [Name, Address]
  - entity: configuration
    shows: [Configuration text]
entryPoints:
  - web-ui: /#/status
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/Status/Views.elm
---

# Status

The instance's uptime, its cluster name, status and peers, its version
information and the configuration it is running with.
