# Secure Alertmanager cluster traffic

Type: Design document

Date: 2019-02-21

Author: Max Inden <IndenML@gmail.com>


## Status Quo

Alertmanager supports [high
availability](https://github.com/prometheus/alertmanager/blob/master/README.md#high-availability)
by interconnecting multiple Alertmanager instances building an Alertmanager
cluster. Instances of a cluster communicate on top of a gossip protocol managed
via Hashicorps [_Memberlist_](https://github.com/hashicorp/memberlist) library.
_Memberlist_ uses two channels to communicate: TCP for reliable and UDP for
best-effort communication.

Alertmanager instances use the gossip layer to:

- Keep track of membership
- Replicate silence creation, update and deletion
- Replicate notification log

As of today the communication between Alertmanager instances in a cluster is
sent in clear-text.


## Goal

Instances in a cluster should communicate among each other in a secure fashion.
Alertmanager should guarantee confidentiality, integrity and client authenticity
for each message touching the wire. While this would improve the security of
single datacenter deployments, one could see this as a necessity for
wide-area-network deployments.


## Non-Goal

Even though solutions might also be applicable to the API endpoints exposed by
Alertmanager, it is not the goal of this design document to secure the API
endpoints.


## Proposed Solution - TLS Memberlist

_Memberlist_ enables users to implement their own [transport
layer](https://godoc.org/github.com/hashicorp/memberlist#Transport) without the
need of forking the library itself. That transport layer needs to support
reliable as well as best-effort communication. Instead of using TCP and UDP like
the default transport layer of _Memberlist_, the suggestion is to only use TCP
for both reliable as well as best-effort communication. On top of that TCP
layer, one can use mutual TLS to secure all communication. A proof-of-concept
implementation can be found here:
https://github.com/mxinden/memberlist-tls-transport.

The data gossiped between instances does not have a low-latency requirement that
TCP could not fulfill, same would apply for the relatively low data throughput
requirements of Alertmanager.

TCP connections could be kept alive beyond a single message to reduce latency as
well as handshake overhead costs. While this is feasible in a 3-instance
Alertmanager cluster, the discussed custom implementation would need to limit
the amount of open connections for clusters with many instances (#connections =
n*(n-1)/2).

As of today, Alertmanager already forces _Memberlist_ to use the reliable TCP
instead of the best-effort UDP connection to gossip large notification logs and
silences between instances. The reason is, that those packets would otherwise
exceed the [MTU](https://en.wikipedia.org/wiki/Maximum_transmission_unit) of
most UDP setups. Splitting packets is not supported by _Memberlist_ and was not
considered worth the effort to be implemented in Alertmanager either. For more
info see this [Github
issue](https://github.com/prometheus/alertmanager/issues/1412).

With the last [Prometheus developer
summit](https://docs.google.com/document/d/1-C5PycocOZEVIPrmM1hn8fBelShqtqiAmFptoG4yK70/edit)
in mind, the Prometheus projects preferred security mechanism seems to be mutual
TLS. Having Alertmanager use the same mechanism would ease deployment with the
rest of the Prometheus stack.

As a side effect (benefit) Alertmanager would only need a single open port (TCP
traffic) instead of two open ports (TCP and UDP traffic) for cluster
communication. This does not affect the API endpoint which remains a separate
TCP port.


## Alternative Solutions

### Symmetric Memberlist

_Memberlist_ supports [symmetric key
encryption](https://godoc.org/github.com/hashicorp/memberlist#Keyring) via
AES-128, AES-192 or AES-256 ciphers. One can specify multiple keys for rolling
updates. Securing the cluster traffic via symmetric encryption would just
involve small configuration changes in the Alertmanager code base.


### Replace Memberlist

Coordinating membership might not be required by the Alertmanager cluster
component. Instead this could be bound to static configuration or e.g. DNS
service discovery. On the other hand, gossiping silences and notifications is
ideally done in an eventual consistent gossip fashion, given that Alertmanager
is supposed to scale beyond a 3-instance cluster and beyond local-area-network
deployments. With these requirements in mind, replacing _Memberlist_ with an
entirely self-built communication layer is a great undertaking.


### TLS Memberlist with DTLS

Instead of redirecting all best-effort traffic via the reliable channel as
proposed above, one could also secure the best-effort channel itself using UDP
and [DTLS](https://en.wikipedia.org/wiki/Datagram_Transport_Layer_Security) in
addition to securing the reliable traffic via TCP and TLS. DTLS is not supported
by the Golang standard library.
