---
availability:
  - { place: api }
references:
  - kind: doc
    role: intent
    target: docs/management_api.md
    title: Management API
  - kind: code
    role: implementation
    target: httpserver/httpserver.go#Register
---

# Check health

Operators and the monitoring and orchestration they set up check that an
Alertmanager is up and ready to serve traffic. Neither check says whether
notifications reach their receivers.
