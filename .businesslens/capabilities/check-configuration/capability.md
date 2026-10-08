---
availability:
  - { place: amtool }
references:
  - kind: code
    role: implementation
    target: cli/check_config.go#CheckConfig
---

# Check configuration

Without a running Alertmanager, an operator checks configuration files the way
Alertmanager would load them, including their notification templates.
