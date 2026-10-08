---
kind: alternative
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator asks for the running Configuration
    kind: actor
    actor: operator
    entities:
      - { entity: configuration, effect: reads, facts: [Configuration text] }
    contexts:
      web:
        place: web-ui::status
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product shows the Configuration as YAML with every secret hidden
    kind: product
    actor: operator
    entities:
      - { entity: configuration, effect: reads, facts: [Configuration text] }
    contexts:
      web:
        place: web-ui::status
      api:
        place: api
      amtool:
        place: amtool
---

# Read the running configuration

## Trigger

An operator checks that a reload took effect.

## Outcome

The operator sees the configuration Alertmanager is running with.
