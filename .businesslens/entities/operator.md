---
kind: person
acts: external
references:
  - kind: doc
    role: context
    target: docs/configuration.md
  - kind: doc
    role: context
    target: docs/alertmanager.md
---

# Operator

A person who works with Alertmanager: watches the alerts it holds, silences
the ones already being handled, checks its status, and runs it — writing its
configuration file, starting, reloading and stopping it, and checking that
routing and templates behave as intended. Alertmanager tells no people apart:
everyone its web configuration admits can do everything.
