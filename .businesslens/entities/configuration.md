---
relations:
  - entity: route
    verb: contains
    cardinality: one-to-many
  - entity: receiver
    verb: defines
    cardinality: one-to-many
  - entity: inhibition-rule
    verb: defines
    cardinality: one-to-many
  - entity: time-interval
    verb: defines
    cardinality: one-to-many
references:
  - kind: code
    role: implementation
    target: config/config.go#Config
  - kind: code
    role: implementation
    target: config/coordinator.go
  - kind: doc
    role: intent
    target: docs/configuration.md
---

# Configuration

The configuration Alertmanager is running with, loaded from the configuration
file the operator names at start: global settings, the routing tree,
receivers, inhibition rules, time intervals, notification template files and
event recorder outputs.

## Information kept

- **Configuration text** — the running configuration as YAML, with every secret replaced by `<secret>`
- **Global settings** — defaults shared by receivers, such as the resolve timeout, SMTP settings and the URLs and credentials of notification services
- **Notification templates** — the template files whose definitions notifications may use
- **Event recorder outputs** — the files, standard output, webhooks and Kafka topics events are recorded to
