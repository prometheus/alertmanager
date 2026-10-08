---
singleton: true
references:
  - kind: doc
    role: intent
    target: docs/https.md
    title: HTTPS and authentication
  - kind: code
    role: implementation
    target: app/lifecycle.go
  - kind: code
    role: implementation
    target: app/listen.go
---

# Web configuration

The web configuration file the operator names at start. Alertmanager reads it
on every HTTP request, so changes apply at once. Every request to the web UI,
the API and the management endpoints must pass it; it admits everyone when it
sets neither basic authentication users nor client certificates.

## Information kept

- **Basic authentication users** — user names and hashed passwords a request must present
- **TLS settings** — the server certificate and whether clients must present a certificate the operator trusts
