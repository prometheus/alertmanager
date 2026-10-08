---
kind: primary
routes:
  web: Web UI
  api: API
  amtool: amtool
steps:
  - text: The Operator asks for the status of the Alertmanager instance
    kind: actor
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Cluster name] }
    contexts:
      web:
        place: web-ui::status
      api:
        place: api
      amtool:
        place: amtool
  - text: The Product shows the Alertmanager instance's version information, uptime, cluster name and status, and the name and address of each Peer
    kind: product
    actor: operator
    entities:
      - { entity: alertmanager-instance, effect: reads, facts: [Version information, Uptime, Cluster name] }
      - { entity: peer, effect: reads, facts: [Name, Address] }
    contexts:
      web:
        place: web-ui::status
      api:
        place: api
      amtool:
        place: amtool
---

# Check cluster and version

## Trigger

An operator wants to know whether the Alertmanager and its cluster are healthy.

## Outcome

The operator sees the instance status and its peers.

## Edge cases

- With clustering off, the cluster status is disabled and no peers are listed.
- amtool warns when the Alertmanager runs a different major or minor version than amtool itself.
