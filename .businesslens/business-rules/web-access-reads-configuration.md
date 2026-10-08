---
appliesTo:
  - type: entity
    id: configuration
    effect: reads
    contexts:
      - { place: api }
      - { place: web-ui }
permits:
  - configuredBy: web-configuration

references:
  - kind: doc
    role: intent
    target: docs/https.md
  - kind: code
    role: implementation
    target: app/lifecycle.go
---

# Only callers the web configuration admits read the running configuration

Every request to the web UI and the API, including those amtool makes, must pass the web configuration: its basic authentication users and client certificates when it sets them, nothing otherwise. Whoever passes may do this, whoever created the thing; Alertmanager keeps no accounts or roles of its own.
