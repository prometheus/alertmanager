---
appliesTo:
  - type: entity
    id: silence
    effect: creates
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

# Only callers the web configuration admits create silences, besides peers through Alertmanager itself

Every request to the web UI and the API, including those amtool makes, must pass the web configuration: its basic authentication users and client certificates when it sets them, nothing otherwise. Whoever passes may do this, whoever created the thing; Alertmanager keeps no accounts or roles of its own. Silences gossiped by peers are kept by Alertmanager itself.
