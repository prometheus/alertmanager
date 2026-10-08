---
kind: system
acts: external
references:
  - kind: doc
    role: intent
    target: docs/alerts_api.md
    title: Alerts API
  - kind: code
    role: implementation
    target: api/v2/api.go#postAlertsHandler
---

# Alert generator

A client application, usually the Prometheus server, that evaluates alerting
rules and sends the resulting alerts to Alertmanager. It is expected to resend
firing alerts at regular intervals until they resolve, and to send each alert
to every Alertmanager of a cluster.
