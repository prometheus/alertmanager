---
entities:
  - entity: alert-group
    shows: [Group labels, Route labels, Receiver, Muted by]
  - entity: alert
    shows: [Labels, Annotations, Starts at, Generator URL, Silenced by, Inhibited by]
  - entity: receiver
    shows: [Name]
entryPoints:
  - web-ui: /#/alerts
references:
  - kind: code
    role: implementation
    target: ui/app/src/Views/AlertList/Views.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/AlertList/AlertView.elm
  - kind: code
    role: implementation
    target: ui/app/src/Views/Shared/Alert.elm
---

# Alerts

The alerts Alertmanager holds, in their alert groups or grouped by labels the
operator picks, narrowed by label matchers, by receiver and by whether
silenced, inhibited or muted alerts are included. Each alert offers to silence
it.
