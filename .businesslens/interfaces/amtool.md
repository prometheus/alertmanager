---
type: cli
actors: [operator]
entryPoints:
  - cli: amtool --alertmanager.url=http://localhost:9093
references:
  - kind: code
    role: implementation
    target: cli/root.go
  - kind: doc
    role: context
    target: cmd/amtool/README.md
---

# amtool

The command-line tool for working with an Alertmanager: querying and adding
alerts, managing silences, showing cluster status and configuration, and,
without a running Alertmanager, checking configuration files, testing routing
and rendering templates. Its defaults can come from a configuration file in
the user's or the system's configuration directory.
