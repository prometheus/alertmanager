---
appliesTo:
  - type: entity
    id: alert
    effect: changes
    contexts:
      - { place: api }
      - { place: web-ui }
      - { place: amtool }
permits:
  - configuredBy: web-configuration
  - unattended: true
references:
  - kind: doc
    role: intent
    target: docs/https.md
  - kind: code
    role: implementation
    target: app/lifecycle.go
---

# Only callers the web configuration admits change alerts, besides Alertmanager itself

Every request to the web UI and the API, including those amtool makes, must pass the web configuration: its basic authentication users and client certificates when it sets them, nothing otherwise. Whoever passes may do this, whoever created the thing; Alertmanager keeps no accounts or roles of its own. Alertmanager itself resolves alerts and marks them silenced or inhibited.
