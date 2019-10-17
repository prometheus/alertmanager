# Clustered Etcd Alert Store

Type: Design Document (and PR)

Date: 2019-09-20

Author: David C Wang <dcwangmit01@gmail.com>

## Status Quo

The current Alertmanager clustering implementation will allow multiple
instances of Alertmanager to sync recently-updated notification log entries and
alert silences via the gossip protocol.  The syncing of alert state is not yet
implemented.

Currently, Alertmanager alert state is entirely stored in process memory.  It
is lost upon restart of Alertmanager.  This, in conjunction with the fact that
Alertmanager alert state is not synced with clustered peers presents several
problems:

* Alert emitters must send alerts to every single Alertmanager instance in the
  cluster.  If some Alertmanager instances receive alerts that other instances
  do not, their state will drift apart.

  * Typically, the alert emitter is Prometheus which uses service discovery to
    locate all Alertmanager instances, and then sends alerts to each individual
    target.  Custom alert emitters may not have that service discovery
    capability and must be explicity configured.

* Because the alert must be sent to every Alertmanager instance, this precludes
  a typical HA deployment pattern of having clients (i.e. alert emitters) send
  alerts through a Load Balancer that fronts several Alertmanager instances.
  This pattern is typical in Kubernetes.

* Restarting Alertmanager instances or increasing the replicaCount for the
  Alertmanager cluster will result in the new members having very different
  alert states than current members.

* If all of the Alertmanagers reboot at the same time, all active alerts are
  lost.  New alerts will be created in each instance memory as alerts come in
  through the API.  However, these alerts will have different startsAt times
  than the prior alerts in memory.  The effect is that "status=resolved"
  notifications will never be sent for the original set of alerts in memory.
  Because some notification receivers consider unique instances of alerts to be
  a combination of alert fingerpint and startsAt, these notification reciverse
  will never receive "status=resolved" notifications.

The issues above may be addressed by implementing a share persistent data store
for alert state.

## Design

How the Etcd Alerts Storage Provider Works

Initialization (Synchronous)

* Upon startup, Alertmanager tries to connect to Etcd.  If the connection
  attempt is unsuccessful after a timeout, then Alertmanager will fail hard
  because it is probably a configuration error.  A supervisor such as
  Kubernetes should restart it.

* Once Alertmanager successfully connects to Etcd, it proceeds with "best
  effort" to write alert updates to Etcd.  Alert updates are the current
  in-memory "running count" state of an alert, and not the log of the entire
  alert stream.

* Alertmanager also initiates an Etcd watch client to receive alert updates
  from Etcd.  As other Alertmanagers in the cluster write to Etcd, all of the
  Alertmanagers receive and process those updates.

* If Etcd disappears for any reason (e.g. network partition, quorum loss,
  etc.), Alertmanager will continue running independently to maintain
  availability.  Once connectivity to Etcd is restored, Alertmanager will
  resume writing alerts to Etcd, and receiving alert updates from Etcd.

* As the last part of initialization, Etcd will read and load all alert objects
  from Etcd.

Ongoing Operation (Async)

* Alertmanager can receive alerts from both API requests AND the Etcd watch
  subscription.  Alerts from both of these sources are sent to the etcd alert
  provider via Put().  To prevent circular loops and promote state convergence,
  the alert is put into Etcd only when there is a difference between the alert
  in memory and the alert in Etcd.

* The Etcd clientv3 library will automatically reconnect if the Etcd server
  goes down or up, or upon resuming network connectivity if a partition had
  occurred.

* The long running watch subscription will reconnect if the watch channel is
  closed.


## Implementation Notes

The "etcd" storage provider is a forked copy of the "mem" storage provider.
Minimum code modifications were made in order to "hook in" the Etcd reads and
writes.

Thus, the following files are nearly similar, and may be combined in the future
if the existing "mem" code may be refactored to support hooks or callbacks:

```
./provider/mem/mem.go -> ./provider/etcd/etcd.go
./provider/mem/mem_test.go -> ./provider/etcd/etcd_test.go
```

Most of the Etcd-specific code are found in the following files:

```
./provider/etcd/etcd_client.go
./provider/etcd/etcd_client_test.go
```

## Demo Notes

Extensive unit tests have been written, but here's how the funcationality may
be demo'd.

```
# Each of the following commands should be run in a separate terminal window so
#   each part of the system may be seen simultaneously.  Use tmux.

# Start (via pre-made docker images)
#   - 1 node etcd cluster (192.168.100.10)
#   - 2 node alertmanager cluster (192.168.100.20, 192.168.100.21)
docker-compose -f docker-compose.etcd.yaml up

# Start an Etcd watch to see alert state writes to Etcd
etcdctl --endpoints=192.168.100.10:2379 watch --prefix am

# Monitor the alert state of each alertmanager instance
watch -n 1 'amtool alert query --alertmanager.url=http://192.168.100.20:9093'
watch -n 1 'amtool alert query --alertmanager.url=http://192.168.100.21:9093'

# Send an alert to one Alertmanager instance
amtool alert add --alertmanager.url=http://192.168.100.20:9093 --start="$(date --iso-8601=seconds)" --end="$(date -d '+1 minute' --iso-8601=seconds)" time="$(date --iso-8601=seconds)"

# Verify
#   The same alert that was sent to the one Alertmanager instance should have
#     shown up in Etcd as well as the other alertmanager instance.
#   After one minute, the alert should have timed out from both alertmanager
#     alert state.
#   Shortly afterwards, the AM garbage collector should kick in and the alert
#     will be deleted from Etcd.

# Cleanup
docker system prune -af
```
