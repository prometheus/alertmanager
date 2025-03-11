# Alertmanager [![CircleCI](https://circleci.com/gh/prometheus/alertmanager/tree/main.svg?style=shield)][circleci]

[![Docker Repository on Quay](https://quay.io/repository/prometheus/alertmanager/status "Docker Repository on Quay")][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/alertmanager.svg?maxAge=604800)][hub]

The Alertmanager handles alerts sent by client applications such as the Prometheus server. It takes care of deduplicating, grouping, and routing them to the correct [receiver integrations](https://prometheus.io/docs/alerting/latest/configuration/#receiver) such as email, PagerDuty, OpsGenie, or many other [mechanisms](https://prometheus.io/docs/operating/integrations/#alertmanager-webhook-receiver) thanks to the webhook receiver. It also takes care of silencing and inhibition of alerts.

* [Documentation](http://prometheus.io/docs/alerting/alertmanager/)

## Install

There are various ways of installing Alertmanager.

### Precompiled binaries

Precompiled binaries for released versions are available in the
[*download* section](https://prometheus.io/download/)
on [prometheus.io](https://prometheus.io). Using the latest production release binary
is the recommended way of installing Alertmanager.

### Docker images

Docker images are available on [Quay.io](https://quay.io/repository/prometheus/alertmanager) or [Docker Hub](https://hub.docker.com/r/prom/alertmanager/).

You can launch an Alertmanager container for trying it out with

    $ docker run --name alertmanager -d -p 127.0.0.1:9093:9093 quay.io/prometheus/alertmanager

Alertmanager will now be reachable at http://localhost:9093/.

### Compiling the binary

You can either `go get` it:

```
$ GO15VENDOREXPERIMENT=1 go get github.com/prometheus/alertmanager/cmd/...
# cd $GOPATH/src/github.com/prometheus/alertmanager
$ alertmanager --config.file=<your_file>
```

Or clone the repository and build manually:

```
$ mkdir -p $GOPATH/src/github.com/prometheus
$ cd $GOPATH/src/github.com/prometheus
$ git clone https://github.com/prometheus/alertmanager.git
$ cd alertmanager
$ make build
$ ./alertmanager --config.file=<your_file>
```

You can also build just one of the binaries in this repo by passing a name to the build function:
```
$ make build BINARIES=amtool
```

## Example

This is an example configuration that should cover most relevant aspects of the new YAML configuration format. The full documentation of the configuration can be found [here](https://prometheus.io/docs/alerting/configuration/).

```yaml
global:
  # The smarthost and SMTP sender used for mail notifications.
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alertmanager@example.org'

# The root route on which each incoming alert enters.
route:
  # The root route must not have any matchers as it is the entry point for
  # all alerts. It needs to have a receiver configured so alerts that do not
  # match any of the sub-routes are sent to someone.
  receiver: 'team-X-mails'

  # The labels by which incoming alerts are grouped together. For example,
  # multiple alerts coming in for cluster=A and alertname=LatencyHigh would
  # be batched into a single group.
  #
  # To aggregate by all possible labels use '...' as the sole label name.
  # This effectively disables aggregation entirely, passing through all
  # alerts as-is. This is unlikely to be what you want, unless you have
  # a very low alert volume or your upstream notification system performs
  # its own grouping. Example: group_by: [...]
  group_by: ['alertname', 'cluster']

  # When a new group of alerts is created by an incoming alert, wait at
  # least 'group_wait' to send the initial notification.
  # This way ensures that you get multiple alerts for the same group that start
  # firing shortly after another are batched together on the first
  # notification.
  group_wait: 30s

  # When the first notification was sent, wait 'group_interval' to send a batch
  # of new alerts that started firing for that group.
  group_interval: 5m

  # If an alert has successfully been sent, wait 'repeat_interval' to
  # resend them.
  repeat_interval: 3h

  # All the above attributes are inherited by all child routes and can
  # overwritten on each.

  # The child route trees.
  routes:
  # This route performs a regular expression match on alert labels to
  # catch alerts that are related to a list of services.
  - matchers:
    - service=~"^(foo1|foo2|baz)$"
    receiver: team-X-mails

    # The service has a sub-route for critical alerts, any alerts
    # that do not match, i.e. severity != critical, fall-back to the
    # parent node and are sent to 'team-X-mails'
    routes:
    - matchers:
      - severity="critical"
      receiver: team-X-pager

  - matchers:
    - service="files"
    receiver: team-Y-mails

    routes:
    - matchers:
      - severity="critical"
      receiver: team-Y-pager

  # This route handles all alerts coming from a database service. If there's
  # no team to handle it, it defaults to the DB team.
  - matchers:
    - service="database"

    receiver: team-DB-pager
    # Also group alerts by affected database.
    group_by: [alertname, cluster, database]

    routes:
    - matchers:
      - owner="team-X"
      receiver: team-X-pager

    - matchers:
      - owner="team-Y"
      receiver: team-Y-pager


# Inhibition rules allow to mute a set of alerts given that another alert is
# firing.
# We use this to mute any warning-level notifications if the same alert is
# already critical.
inhibit_rules:
- source_matchers:
    - severity="critical"
  target_matchers:
    - severity="warning"
  # Apply inhibition if the alertname is the same.
  # CAUTION: 
  #   If all label names listed in `equal` are missing 
  #   from both the source and target alerts,
  #   the inhibition rule will apply!
  equal: ['alertname']


receivers:
- name: 'team-X-mails'
  email_configs:
  - to: 'team-X+alerts@example.org, team-Y+alerts@example.org'

- name: 'team-X-pager'
  email_configs:
  - to: 'team-X+alerts-critical@example.org'
  pagerduty_configs:
  - routing_key: <team-X-key>

- name: 'team-Y-mails'
  email_configs:
  - to: 'team-Y+alerts@example.org'

- name: 'team-Y-pager'
  pagerduty_configs:
  - routing_key: <team-Y-key>

- name: 'team-DB-pager'
  pagerduty_configs:
  - routing_key: <team-DB-key>
```

## API

The current Alertmanager API is version 2. This API is fully generated via the
[OpenAPI project](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/2.0.md)
and [Go Swagger](https://github.com/go-swagger/go-swagger/) with the exception
of the HTTP handlers themselves. The API specification can be found in
[api/v2/openapi.yaml](api/v2/openapi.yaml). A HTML rendered version can be
accessed [here](http://petstore.swagger.io/?url=https://raw.githubusercontent.com/prometheus/alertmanager/main/api/v2/openapi.yaml).
Clients can be easily generated via any OpenAPI generator for all major languages.

APIv2 is accessed via the `/api/v2` prefix. APIv1 was deprecated in `0.16.0` and is removed as of version `0.27.0`.
The v2 `/status` endpoint would be `/api/v2/status`. If `--web.route-prefix` is set then API routes are
prefixed with that as well, so `--web.route-prefix=/alertmanager/` would
relate to `/alertmanager/api/v2/status`.

## amtool

`amtool` is a cli tool for interacting with the Alertmanager API. It is bundled with all releases of Alertmanager.

### Install

Alternatively you can install with:
```
$ go install github.com/prometheus/alertmanager/cmd/amtool@latest
```

### Examples

View all currently firing alerts:
```
$ amtool alert
Alertname        Starts At                Summary
Test_Alert       2017-08-02 18:30:18 UTC  This is a testing alert!
Test_Alert       2017-08-02 18:30:18 UTC  This is a testing alert!
Check_Foo_Fails  2017-08-02 18:30:18 UTC  This is a testing alert!
Check_Foo_Fails  2017-08-02 18:30:18 UTC  This is a testing alert!
```

View all currently firing alerts with extended output:
```
$ amtool -o extended alert
Labels                                        Annotations                                                    Starts At                Ends At                  Generator URL
alertname="Test_Alert" instance="node0"       link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
alertname="Test_Alert" instance="node1"       link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
alertname="Check_Foo_Fails" instance="node0"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
alertname="Check_Foo_Fails" instance="node1"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
```

In addition to viewing alerts, you can use the rich query syntax provided by Alertmanager:
```
$ amtool -o extended alert query alertname="Test_Alert"
Labels                                   Annotations                                                    Starts At                Ends At                  Generator URL
alertname="Test_Alert" instance="node0"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
alertname="Test_Alert" instance="node1"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local

$ amtool -o extended alert query instance=~".+1"
Labels                                        Annotations                                                    Starts At                Ends At                  Generator URL
alertname="Test_Alert" instance="node1"       link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
alertname="Check_Foo_Fails" instance="node1"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local

$ amtool -o extended alert query alertname=~"Test.*" instance=~".+1"
Labels                                   Annotations                                                    Starts At                Ends At                  Generator URL
alertname="Test_Alert" instance="node1"  link="https://example.com" summary="This is a testing alert!"  2017-08-02 18:31:24 UTC  0001-01-01 00:00:00 UTC  http://my.testing.script.local
```

Silence an alert:
```
$ amtool silence add alertname=Test_Alert
b3ede22e-ca14-4aa0-932c-ca2f3445f926

$ amtool silence add alertname="Test_Alert" instance=~".+0"
e48cb58a-0b17-49ba-b734-3585139b1d25
```

View silences:
```
$ amtool silence query
ID                                    Matchers              Ends At                  Created By  Comment
b3ede22e-ca14-4aa0-932c-ca2f3445f926  alertname=Test_Alert  2017-08-02 19:54:50 UTC  kellel

$ amtool silence query instance=~".+0"
ID                                    Matchers                            Ends At                  Created By  Comment
e48cb58a-0b17-49ba-b734-3585139b1d25  alertname=Test_Alert instance=~.+0  2017-08-02 22:41:39 UTC  kellel
```

Expire a silence:
```
$ amtool silence expire b3ede22e-ca14-4aa0-932c-ca2f3445f926
```

Expire all silences matching a query:
```
$ amtool silence query instance=~".+0"
ID                                    Matchers                            Ends At                  Created By  Comment
e48cb58a-0b17-49ba-b734-3585139b1d25  alertname=Test_Alert instance=~.+0  2017-08-02 22:41:39 UTC  kellel

$ amtool silence expire $(amtool silence query -q instance=~".+0")

$ amtool silence query instance=~".+0"

```

Expire all silences:
```
$ amtool silence expire $(amtool silence query -q)
```

Try out how a template works. Let's say you have this in your configuration file:
```
templates:
  - '/foo/bar/*.tmpl'
```

Then you can test out how a template would look like with example by using this command:
```
amtool template render --template.glob='/foo/bar/*.tmpl' --template.text='{{ template "slack.default.markdown.v1" . }}'
```

### Configuration

`amtool` allows a configuration file to specify some options for convenience. The default configuration file paths are `$HOME/.config/amtool/config.yml` or `/etc/amtool/config.yml`

An example configuration file might look like the following:

```
# Define the path that `amtool` can find your `alertmanager` instance
alertmanager.url: "http://localhost:9093"

# Override the default author. (unset defaults to your username)
author: me@example.com

# Force amtool to give you an error if you don't include a comment on a silence
comment_required: true

# Set a default output format. (unset defaults to simple)
output: extended

# Set a default receiver
receiver: team-X-pager
```

### Routes

`amtool` allows you to visualize the routes of your configuration in form of text tree view.
Also you can use it to test the routing by passing it label set of an alert
and it prints out all receivers the alert would match ordered and separated by `,`.
(If you use `--verify.receivers` amtool returns error code 1 on mismatch)

Example of usage:
```
# View routing tree of remote Alertmanager
$ amtool config routes --alertmanager.url=http://localhost:9090

# Test if alert matches expected receiver
$ amtool config routes test --config.file=doc/examples/simple.yml --tree --verify.receivers=team-X-pager service=database owner=team-X
```

## High Availability

Alertmanager's high availability is in production use at many companies and is enabled by default.

> Important: Both UDP and TCP are needed in alertmanager 0.15 and higher for the cluster to work.
>  - If you are using a firewall, make sure to whitelist the clustering port for both protocols.
>  - If you are running in a container, make sure to expose the clustering port for both protocols.

To create a highly available cluster of the Alertmanager the instances need to
be configured to communicate with each other. This is configured using the
`--cluster.*` flags.

- `--cluster.listen-address` string: cluster listen address (default "0.0.0.0:9094"; empty string disables HA mode)
- `--cluster.advertise-address` string: cluster advertise address
- `--cluster.peer` value: initial peers (repeat flag for each additional peer)
- `--cluster.peer-timeout` value: peer timeout period (default "15s")
- `--cluster.gossip-interval` value: cluster message propagation speed
  (default "200ms")
- `--cluster.pushpull-interval` value: lower values will increase
  convergence speeds at expense of bandwidth (default "1m0s")
- `--cluster.settle-timeout` value: maximum time to wait for cluster
  connections to settle before evaluating notifications.
- `--cluster.tcp-timeout` value: timeout value for tcp connections, reads and writes (default "10s")
- `--cluster.probe-timeout` value: time to wait for ack before marking node unhealthy
  (default "500ms")
- `--cluster.probe-interval` value: interval between random node probes (default "1s")
- `--cluster.reconnect-interval` value: interval between attempting to reconnect to lost peers (default "10s")
- `--cluster.reconnect-timeout` value: length of time to attempt to reconnect to a lost peer (default: "6h0m0s")
- `--cluster.label` value: the label is an optional string to include on each packet and stream. It uniquely identifies the cluster and prevents cross-communication issues when sending gossip messages (default:"")

The chosen port in the `cluster.listen-address` flag is the port that needs to be
specified in the `cluster.peer` flag of the other peers.

The `cluster.advertise-address` flag is required if the instance doesn't have
an IP address that is part of [RFC 6890](https://tools.ietf.org/html/rfc6890)
with a default route.

To start a cluster of three peers on your local machine use [`goreman`](https://github.com/mattn/goreman) and the
Procfile within this repository.

	goreman start

To point your Prometheus 1.4, or later, instance to multiple Alertmanagers, configure them
in your `prometheus.yml` configuration file, for example:

```yaml
alerting:
  alertmanagers:
  - static_configs:
    - targets:
      - alertmanager1:9093
      - alertmanager2:9093
      - alertmanager3:9093
```

> Important: Do not load balance traffic between Prometheus and its Alertmanagers, but instead point Prometheus to a list of all Alertmanagers. The Alertmanager implementation expects all alerts to be sent to all Alertmanagers to ensure high availability.

### Turn off high availability

If running Alertmanager in high availability mode is not desired, setting `--cluster.listen-address=` prevents Alertmanager from listening to incoming peer requests.

## Contributing

Check the [Prometheus contributing page](https://github.com/prometheus/prometheus/blob/main/CONTRIBUTING.md).

To contribute to the user interface, refer to [ui/app/CONTRIBUTING.md](ui/app/CONTRIBUTING.md).

## Architecture

![](doc/arch.svg)

## License

Apache License 2.0, see [LICENSE](https://github.com/prometheus/alertmanager/blob/main/LICENSE).

[hub]: https://hub.docker.com/r/prom/alertmanager/
[circleci]: https://circleci.com/gh/prometheus/alertmanager
[quay]: https://quay.io/repository/prometheus/alertmanager
