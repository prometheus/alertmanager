---
appliesTo:
  - { type: capability, id: view-configuration }
  - type: entity
    id: configuration
    effect: reads
    facts: [Configuration text]
references:
  - kind: code
    role: implementation
    target: config/config.go
---

# Secrets in the configuration are never shown

Passwords, tokens, API keys and secret URLs appear as `<secret>` wherever the
running configuration is shown.
