# Protect Alertmanager cluster from foreign membership

Type: Design document

Date: 2020-03-08

Author: Holger Hans Peter Freyther <automatic+am@freyther.de>

Status: Draft

## Status Quo

Alertmanager supports [high
availability](https://github.com/prometheus/alertmanager/blob/master/README.md#high-availability)
by interconnecting multiple Alertmanager instances building an Alertmanager
cluster. Instances of a cluster communicate on top of a gossip protocol managed
via Hashicorps [_Memberlist_](https://github.com/hashicorp/memberlist) library.
_Memberlist_ uses two channels to communicate: TCP for reliable and UDP for
best-effort communication.

Today knowing the address of any peer is enough to join the cluster and
(accidentally) gossip silences and the alert notification log.


## Goal

Prevent non-production Alertmanager instances to accidentally gossip silences
to members of a production cluster.

## Proposed Solution - Memberlist Keys/Keyring

Hashicorps [_Memberlist_](https://github.com/hashicorp/memberlist) allows to
manage a _Keyring_ with one or more keys and encrypt outgoing messages and
verify encryption of Gossip messages received. Enabling encryption has an
impact on the size of messages exchanged and requires extra compute.

Introduce the  _cluster.key-file_ command line to specify zero to many files
containing encryption keys to be used as keys in the [_Memberlist_]. The first
key specified will be the primary key and enable the protection of the cluster.

Keys can be rotated by adding an additional _cluster.key-file_ and restart all
all instances of the cluster and then remove the old key.


## Discarded Solutions

### Implement the secure cluster traffic document

Implementing and operating a X509 PKI is a major challenge. An implementation
must honor certificate expiration, check revocation lists/OCSP and many more
details. Operating a PKI is equally challenging and many high profile companies
fail[1][2][3] at the basics. A more manageable solution is preferable.


[1] https://www.theverge.com/2020/2/3/21120248/microsoft-teams-down-outage-certificate-issue-status
[2] https://www.zdnet.com/article/ericsson-expired-certificate-caused-o2-and-softbank-outages/
[3] https://www.theregister.co.uk/2017/11/30/linkedin_ssl_certificates_expire/

