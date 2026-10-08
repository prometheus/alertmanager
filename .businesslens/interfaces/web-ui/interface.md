---
type: web
actors: [operator]
entryPoints:
  - web: /#/alerts
references:
  - kind: code
    role: implementation
    target: ui/web.go
  - kind: code
    role: implementation
    target: ui/app/src/Parsing.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/NavBar/Types.elm
---

# Web UI

The browser interface Alertmanager serves at its root: operators look through
alerts and their groups, create, edit and expire silences, see the instance's
status and running configuration, and choose browser settings.
