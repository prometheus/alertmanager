---
domain: alerts
availability:
  - { place: web-ui }
  - { place: api }
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: api/v2/api.go#getAlertGroupsHandler
  - kind: code
    role: implementation
    target: api/v2/api.go#getAlertsHandler
  - kind: code
    role: implementation
    target: ui/app/src/Views/AlertList/Updates.elm
  - kind: code
    role: implementation
    target: cli/alert_query.go
---

# Browse alerts

Operators look through the unresolved alerts Alertmanager holds, either in
their alert groups or one by one, narrowed by label matchers, by receiver and
by whether silenced, inhibited or muted alerts are included. Each alert shows
what silences or inhibits it at the moment it is listed, and each group which
time intervals mute it.
