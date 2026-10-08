---
type: cli
actors: [operator]
entryPoints:
  - cli: alertmanager --config.file=alertmanager.yml
references:
  - kind: code
    role: implementation
    target: cmd/alertmanager/main.go
  - kind: code
    role: implementation
    target: app/lifecycle.go
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# Alertmanager server

The `alertmanager` command operators run: started with a configuration file
and command-line settings for storage, retention, limits, the web listener and
clustering, told to reload its configuration with a hang-up signal and to stop
with a termination signal. Its logs and metrics show operators how
notification delivery is going.
