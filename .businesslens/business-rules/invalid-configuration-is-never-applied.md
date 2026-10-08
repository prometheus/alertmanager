---
appliesTo:
  - { type: capability, id: start-alertmanager }
  - { type: capability, id: reload-configuration }
  - { type: capability, id: check-configuration }
  - { type: entity, id: configuration }
references:
  - kind: code
    role: implementation
    target: config/config.go
  - kind: code
    role: implementation
    target: config/coordinator.go
  - kind: code
    role: implementation
    target: app/reloader.go
---

# A configuration that fails its checks is never applied

Among the checks: the root route has a receiver and no matchers or time
intervals, every route names a defined receiver and defined time intervals,
receiver and time interval names are unique, grouping by every label is not
mixed with named labels, group and repeat intervals are not zero, and every
template and integration can be prepared. A failing file stops Alertmanager
from starting, and on reload the running configuration, alerts, silences and
notification log stay as they were.
