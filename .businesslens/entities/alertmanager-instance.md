---
relations:
  - entity: peer
    verb: gossips with
    cardinality: one-to-many
references:
  - kind: code
    role: implementation
    target: cluster/cluster.go
  - kind: code
    role: implementation
    target: api/v2/api.go#getStatusHandler
  - kind: code
    role: implementation
    target: httpserver/httpserver.go
---

# Alertmanager instance

The running Alertmanager process: its build, how long it has run, its place in
a cluster and how it reports its own health.

## Information kept

- **Version information** — version, revision, branch, build user, build date and Go version
- **Uptime** — when it started
- **Cluster name** — its own name among the peers
- **Health** — whether it is up, answered while it serves HTTP
- **Readiness** — whether it is ready to serve traffic
- **Metrics** — measurements of alerts, notifications, silences, reloads, clustering and requests

## States

### Settling

Clustering is on and it is still waiting for the membership of the cluster to
settle; it receives alerts and serves the API but holds back notifications.

### Ready

Clustering is on and the cluster has settled.

### Disabled

Clustering is off; it runs alone.
