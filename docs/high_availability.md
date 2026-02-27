---
title: High Availability
sort_rank: 4
nav_icon: network
---

Alertmanager supports configuration to create a cluster for high availability. This document describes how the HA mechanism works, its design goals, and operational considerations.

## Design Goals

The Alertmanager HA implementation is designed around three core principles:

1. **Single pane view and management** - Silences and alerts can be viewed and managed from any cluster member, providing a unified operational experience
2. **Survive cluster split-brain with "fail open"** - During network partitions, Alertmanager prefers to send duplicate notifications rather than miss critical alerts
3. **At-least-once delivery** - The system guarantees that notifications are delivered at least once, in line with the fail-open philosophy

These goals prioritize operational reliability and alert delivery over strict exactly-once semantics.

## Architecture Overview

An Alertmanager cluster consists of multiple Alertmanager instances that communicate using a gossip protocol. Each instance:

- Receives alerts independently from Prometheus servers
- Participates in a peer-to-peer gossip mesh
- Replicates state (silences and notification log) to other cluster members
- Processes and sends notifications independently

```
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│ Prometheus 1 │    │ Prometheus 2 │    │ Prometheus N │
└──────┬───────┘    └──────┬───────┘    └──────┬───────┘
       │                   │                   │
       │ alerts            │ alerts            │ alerts
       │                   │                   │
       ▼                   ▼                   ▼
    ┌────────────────────────────────────────────┐
    │  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
    │  │  AM-1    │  │  AM-2    │  │  AM-3    │  │
    │  │ (pos: 0) ├──┤ (pos: 1) ├──┤ (pos: 2) │  │
    │  └──────────┘  └──────────┘  └──────────┘  │
    │          Gossip Protocol (Memberlist)      │
    └────────────────────────────────────────────┘
              │           │           │
              ▼           ▼           ▼
         Receivers   Receivers   Receivers
```

## Gossip Protocol

Alertmanager uses [Hashicorp's Memberlist](https://github.com/hashicorp/memberlist) library to implement gossip-based communication. The gossip protocol handles:

### Membership Management

- **Automatic peer discovery** - Instances can be configured with a list of known peers and will automatically discover other cluster members
- **Health checking** - Regular probes detect failed members (default: every 1 second)
- **Failure detection** - Failed members are marked and can attempt to rejoin

### State Replication

The gossip layer replicates three types of state:

1. **Silences** - Create, update, and delete operations are broadcast to all peers
2. **Notification log** - Records of which notifications were sent to prevent duplicates
3. **Membership changes** - Join, leave, and failure events

State is eventually consistent - all cluster members will converge to the same state given sufficient time and network connectivity.

### Gossip Settling

When an Alertmanager starts or rejoins the cluster, it waits for gossip to "settle" before processing notifications. This prevents sending notifications based on incomplete state.

The settling algorithm waits until:
- The number of peers remains stable for 3 consecutive checks (default interval: push-pull interval)
- Or a timeout occurs (configurable via context)

During this time, the instance already receives and stores alerts but defers notification processing.

## Notification Pipeline in HA Mode

The notification pipeline operates differently in a clustered environment to ensure deduplication while maintaining at-least-once delivery:

```
┌────────────────────────────────────────────────┐
│              DISPATCHER STAGE                  │
├────────────────────────────────────────────────┤
│ 1. Find matching route(s)                      │
│ 2. Find/create aggregation group within route  │
│ 3. Throttle by group wait or group interval    │
└───────────────────┬────────────────────────────┘
                    │
                    ▼
┌────────────────────────────────────────────────┐
│               NOTIFIER STAGE                   │
├────────────────────────────────────────────────┤
│ 1. Wait for HA gossip to settle                │◄─── Ensures complete state
│ 2. Filter inhibited alerts                     │
│ 3. Filter non-time-active alerts               │
│ 4. Filter time-muted alerts                    │
│ 5. Filter silenced alerts                      │◄─── Uses replicated silences
│ 6. Wait according to HA cluster peer index     │◄─── Staggered notifications
│ 7. Dedupe by repeat interval/HA state          │◄─── Uses notification log
│ 8. Notify & retry intermittent failures        │
│ 9. Update notification log                     │◄─── Replicated to peers
└────────────────────────────────────────────────┘
```

### HA-Specific Stages

#### 1. Gossip Settling Wait

Before the first notification from a group, the instance waits for gossip to settle. This ensures:
- Silences are fully replicated
- The notification log contains recent send records from other instances
- The cluster membership is stable

**Implementation**: `peer.WaitReady(ctx)`

#### 2. Peer Position-Based Wait

To prevent all cluster members from sending notifications simultaneously, each instance waits based on its position in the sorted peer list:

```
wait_time = peer_position × peer_timeout
```

For example, with 3 instances and a 15-second peer timeout:
- Instance `am-1` (position 0): waits 0 seconds
- Instance `am-2` (position 1): waits 15 seconds
- Instance `am-3` (position 2): waits 30 seconds

This staggered timing allows:
- The first instance to send the notification
- Subsequent instances to see the notification log entry
- Deduplication to prevent duplicate sends

**Implementation**: `clusterWait()` in `cmd/alertmanager/main.go:594`

Position is determined by sorting all peer names alphabetically:

```go
func (p *Peer) Position() int {
    all := p.mlist.Members()
    sort.Slice(all, func(i, j int) bool {
        return all[i].Name < all[j].Name
    })
    // Find position of self in sorted list
}
```

#### 3. Deduplication via Notification Log

The `DedupStage` queries the notification log to determine if a notification should be sent:

```go
// Check notification log for recent sends
entry := nflog.Query(receiver, groupKey)
if entry.exists && !shouldNotify(entry, alerts, repeatInterval) {
    // Skip: already notified recently
    return nil
}
```

Deduplication checks:
- **Firing alerts changed?** If yes, notify
- **Resolved alerts changed?** If yes and `send_resolved: true`, notify
- **Repeat interval elapsed?** If yes, notify
- **Otherwise**: Skip notification (deduplicated)

The notification log is replicated via gossip, so all cluster members share the same send history.

## Split-Brain Handling (Fail Open)

During a network partition, the cluster may split into multiple groups that cannot communicate. Alertmanager's "fail open" design ensures alerts are still delivered:

### Scenario: Network Partition

```
Before partition:
┌────────┬────────┬────────┐
│  AM-1  │  AM-2  │  AM-3  │
└────────┴────────┴────────┘
    Unified cluster

After partition:
┌────────┐       │       ┌────────┬────────┐
│  AM-1  │       │       │  AM-2  │  AM-3  │
└────────┘       │       └────────┴────────┘
 Partition A     │        Partition B
```

### Behavior During Partition

**In Partition A** (AM-1 alone):
- AM-1 sees itself as position 0
- Waits 0 × timeout = 0 seconds
- Sends notifications (no dedup from AM-2/AM-3)

**In Partition B** (AM-2, AM-3):
- AM-2 is position 0, AM-3 is position 1
- AM-2 waits 0 seconds, sends notification
- AM-3 sees AM-2's notification log entry, deduplicates

**Result**: Duplicate notifications sent (one from Partition A, one from Partition B)

This is **intentional** - Alertmanager prefers duplicate notifications over missed alerts.

### After Partition Heals

When the network partition heals:
1. Gossip protocol detects all peers again
2. Notification logs are merged (via CRDT-like merge with timestamp)
3. Future notifications are deduplicated correctly across all instances
4. Silences created in either partition are replicated to all peers

## Silence Management in HA

Silences are first-class replicated state in the cluster.

### Silence Creation and Updates

When a silence is created or updated on any instance:

1. **Local storage** - Silence is stored in the local state map
2. **Broadcast** - Silence is serialized (protobuf) and broadcast via gossip
3. **Merge on receive** - Other instances receive and merge the silence:
   ```go
   // Merge logic: last-write-wins based on UpdatedAt timestamp
   if !exists || incoming.UpdatedAt > existing.UpdatedAt {
       accept_update()
   }
   ```
4. **Indexing** - The silence matcher cache is updated for fast alert matching

### Silence Expiry

Silences have:
- `StartsAt`, `EndsAt` - The active time range
- `ExpiresAt` - When to garbage collect (EndsAt + retention period)
- `UpdatedAt` - For conflict resolution during merge

Each instance independently:
- Evaluates silence state (pending/active/expired) based on current time
- Garbage collects expired silences past their retention period
- The GC is local only (no gossip) since all instances converge to the same decision

### Single Pane of Glass

Users can interact with any Alertmanager instance in the cluster:
- **View silences** - All instances have the same silence state (eventually consistent)
- **Create/update silences** - Changes made on any instance propagate to all peers
- **Delete silences** - Implemented as "expire immediately" + gossip

This provides a unified operational experience regardless of which instance you access.

## Operational Considerations

### Configuration

To configure a cluster, each Alertmanager instance needs:

```yaml
# alertmanager.yml
global:
  # ... other config ...

# No cluster config in YAML - use CLI flags
```

Command-line flags:

```bash
alertmanager \
  --cluster.listen-address=0.0.0.0:9094 \
  --cluster.peer=am-1.example.com:9094 \
  --cluster.peer=am-2.example.com:9094 \
  --cluster.peer=am-3.example.com:9094 \
  --cluster.advertise-address=$(hostname):9094 \
  --cluster.peer-timeout=15s \
  --cluster.gossip-interval=200ms \
  --cluster.pushpull-interval=60s
```

Key flags:
- `--cluster.listen-address` - Bind address for cluster communication (default: `0.0.0.0:9094`)
- `--cluster.peer` - List of peer addresses (can be repeated)
- `--cluster.advertise-address` - Address advertised to peers (auto-detected if omitted)
- `--cluster.peer-timeout` - Wait time per peer position for deduplication (default: `15s`)
- `--cluster.gossip-interval` - How often to gossip (default: `200ms`)
- `--cluster.pushpull-interval` - Full state sync interval (default: `60s`)
- `--cluster.probe-interval` - Peer health check interval (default: `1s`)
- `--cluster.settle-timeout` - Max time to wait for gossip settling (default: context timeout)

### Prometheus Configuration

**Important**: Configure Prometheus to send alerts to **all** Alertmanager instances, not via a load balancer.

```yaml
# prometheus.yml
alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - am-1.example.com:9093
            - am-2.example.com:9093
            - am-3.example.com:9093
```

This ensures:
- **Redundancy** - If one Alertmanager is down, others still receive alerts
- **Independent processing** - Each instance independently evaluates routing, grouping, and deduplication
- **No single point of failure** - Load balancers introduce a single point of failure

### Cluster Size Considerations

Since Alertmanager uses gossip without quorum or voting, **any N instances tolerate up to N-1 failures** - as long as one instance is alive, notifications will be sent.

However, cluster size involves tradeoffs:

**Benefits of more instances:**
- Greater resilience to simultaneous failures (hardware, network, datacenter outages)
- Continued operation even during maintenance windows

**Costs of more instances:**
- In case of partitions there will be an increase in duplicate notifications
- More gossip traffic

**Typical deployments:**
- **2-3 instances** - Common for single-datacenter production deployments
- **4-5 instances** - Multi-datacenter or highly critical environments

**Note**: Unlike consensus-based systems (etcd, Raft), odd vs. even cluster sizes make no difference - there is no voting or quorum.

### Monitoring Cluster Health

Key metrics to monitor:

```
# Cluster size
alertmanager_cluster_members

# Peer health
alertmanager_cluster_peer_info

# Peer position (affects notification timing)
alertmanager_peer_position

# Failed peers
alertmanager_cluster_failed_peers

# State replication
alertmanager_nflog_gossip_messages_propagated_total
alertmanager_silences_gossip_messages_propagated_total
```

### Security

By default, cluster communication is unencrypted. For production deployments, especially across WANs, use mutual TLS:

```bash
alertmanager \
  --cluster.tls-config=/etc/alertmanager/cluster-tls.yml
```

See [Secure Cluster Traffic](../doc/design/secure-cluster-traffic.md) for details.

### Persistence

Each Alertmanager instance persists:
- **Silences** - Stored in a snapshot file (default: `data/silences`)
- **Notification log** - Stored in a snapshot file (default: `data/nflog`)

On restart:
1. Instance loads silences and notification log from disk
2. Joins the cluster and gossips with peers
3. Merges state received from peers (newer timestamps win)
4. Begins processing notifications after gossip settling

**Note**: Alerts themselves are **not** persisted - Prometheus re-sends firing alerts regularly.

### Common Pitfalls

1. **Load balancing Prometheus → Alertmanager**
   - ❌ Don't use a load balancer
   - ✅ Configure all instances in Prometheus

2. **Not waiting for gossip to settle**
   - Can lead to missed silences or duplicate notifications on startup
   - The `--cluster.settle-timeout` flag controls this

3. **Network ACLs blocking cluster port**
   - Ensure port 9094 (or your `--cluster.listen-address` port) is open between all instances
   - Both TCP and UDP are used by default (TCP only if using TLS transport)

4. **Unroutable advertise addresses**
   - If `--cluster.advertise-address` is not set, Alertmanager tries to auto-detect
   - For cloud/NAT environments, explicitly set a routable address

5. **Mismatched cluster configurations**
   - All instances should have the same `--cluster.peer-timeout` and gossip settings
   - Mismatches can cause unnecessary duplicates or missed notifications

## How It Works: End-to-End Example

### Scenario: 3-instance cluster, new alert group

1. **Alert arrives** at all 3 instances from Prometheus
2. **Dispatcher** creates aggregation group, waits `group_wait` (e.g., 30s)
3. **After group_wait**:
   - Each instance prepares to notify
4. **Notifier stage**:
   - All instances wait for gossip to settle (if just started)
   - **AM-1** (position 0): waits 0s, checks notification log (empty), sends notification, logs to nflog
   - **AM-2** (position 1): waits 15s, checks notification log (sees AM-1's entry), **skips** notification
   - **AM-3** (position 2): waits 30s, checks notification log (sees AM-1's entry), **skips** notification
5. **Result**: Exactly one notification sent (by AM-1)

### Scenario: AM-1 fails

1. **Alert arrives** at AM-2 and AM-3 only
2. **Dispatcher** creates group, waits `group_wait`
3. **Notifier stage**:
   - AM-1 is not in cluster (failed probe)
   - **AM-2** is now position 0: waits 0s, sends notification
   - **AM-3** is now position 1: waits 15s, sees AM-2's entry, skips
4. **Result**: Notification still sent (fail-open)

### Scenario: Network partition during notification

1. **Alert arrives** at all instances
2. **Network partition** splits AM-1 from AM-2/AM-3
3. **In partition A** (AM-1):
   - Position 0, waits 0s, sends notification
4. **In partition B** (AM-2, AM-3):
   - AM-2 is position 0, waits 0s, sends notification
   - AM-3 is position 1, waits 15s, deduplicates
5. **Result**: Two notifications sent (one per partition) - fail-open behavior

## Troubleshooting

### Check cluster status

```bash
# View cluster members via API
curl http://am-1:9093/api/v2/status

# Check metrics
curl http://am-1:9093/metrics | grep cluster
```

### Diagnose split-brain

If you suspect split-brain:

1. Check `alertmanager_cluster_members` on each instance
   - Should match total cluster size
2. Check `alertmanager_cluster_peer_info{state="alive"}`
   - Should show all peers as alive
3. Review network connectivity between instances

### Debug duplicate notifications

Duplicate notifications can occur due to:

1. **Network partitions** (expected, fail-open)
2. **Gossip not settled** - Check `--cluster.settle-timeout`
3. **Clock skew** - Ensure NTP is configured on all instances
4. **Notification log not replicating** - Check gossip metrics

Enable debug logging:

```bash
alertmanager --log.level=debug
```

Look for:
- `"Waiting for gossip to settle..."`
- `"gossip settled; proceeding"`
- Deduplication decisions in notification pipeline

## Further Reading

- [Alertmanager Configuration](configuration.md)
- [Secure Cluster Traffic Design](../doc/design/secure-cluster-traffic.md)
- [Hashicorp Memberlist Documentation](https://github.com/hashicorp/memberlist)
